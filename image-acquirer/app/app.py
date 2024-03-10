import base64
import logging
import sys
import os
import threading
import json
import time
import math

import cv2 as cv2
from flask import Flask, redirect, Response
from announcer import MessageAnnouncer
from ultralytics import YOLO
import torch

import paho.mqtt.client as mqtt

# do not log access to health probes
class LogFilter(logging.Filter):
    def filter(self, record):
        msg = record.getMessage()
        if "/livez" in msg or "/readyz" in msg: return False
        return True
logging.getLogger("werkzeug").addFilter(LogFilter())
logging.basicConfig(
    format='%(asctime)s %(levelname)-8s %(message)s',
    level=logging.INFO,
    datefmt='%Y-%m-%d %H:%M:%S'
)

def mqtt_on_connect(client, userdata, flags, reason_code, properties):
    print(f'mqtt connected with result code {reason_code}')

class InterestingObjects:
    max_objects = 10
    object_ids = []
    interested_classes = []
    count = 0

    def __init__(self, interested_classes):
        self.interested_classes = interested_classes

    # returns True if a new object is detected
    def update(self, boxes) -> bool:
        if (boxes.id is None) or (boxes.cls is None):
            return False

        cls = boxes.cls.numpy().astype(int)
        ids = boxes.id.numpy().astype(int)
        new_object_detected = False
        for idx, cl in enumerate(cls):
            if cl not in self.interested_classes:
                continue

            object_id = ids[idx]
            known_id = False
            for id in self.object_ids:
                if id == object_id:
                    known_id = True
                    break

            if known_id:
                continue

            # we found a new object
            new_object_detected = True
            self.count += + 1
            logging.info(f"object id = {object_id}")
            self.object_ids.append(object_id)
            if len(self.object_ids) > self.max_objects:
                self.object_ids.pop(0)

        return new_object_detected

app = Flask(__name__, static_url_path='')

def stop_detection_task():
    logging.info("notifying background thread")
    continue_running.clear()
    background_thread.join()
    logging.info("background thread exited cleanly")

def detection_task(camera_device, force_cpu, interested_classes, mqttc, mqtt_topic):
    retry = 500
    interesting_objects = InterestingObjects(interested_classes)

    accel_device = "cpu"
    if force_cpu:
        logging.info("forcing CPU inferencing")
    else:
        if torch.cuda.is_available():
            logging.info("CUDA is available")
            torch.cuda.set_device(0)
            accel_device = "cuda"
        elif torch.backends.mps.is_available():
            logging.info("MPS is available")
            accel_device = "mps"
        else:
            logging.info("CUDA and MPS are not available")

    logging.info("loading model...")
    model = YOLO('best.pt')
    logging.info("done loading model")

    torch.device(accel_device)
    if accel_device != "cpu":
        logging.info(f"moving model to {accel_device}")
        model.to(accel_device)

    total_paint_leaks = 0

    cam = cv2.VideoCapture(camera_device)
    retry_pause = False

    while continue_running.is_set():
        result, frame = cam.read()
        if not result:
            logging.info("Video source did not return a frame")
            cam.open(camera_device)
            if retry_pause:
                time.sleep(2)
            else:
                retry_pause = True
            continue

        retry_pause = False
        results = model.track(frame, persist=True, device=accel_device)
        if len(results) < 1:
            continue

        result = results[0]
        inference_speed = None
        if result.speed is not None:
            inference_speed = result.speed.get('inference')

        output_frame = result.plot()
        # convert image to base64-encoded JPEG
        im_encoded = cv2.imencode('.jpg', output_frame)[1]
        im_b64 = base64.b64encode(im_encoded.tobytes()).decode('ascii')

        if interesting_objects.update(result.boxes) and mqttc is not None:
            frame_encoded = cv2.imencode('.jpg', frame)[1]
            frame_b64 = base64.b64encode(frame_encoded.tobytes()).decode('ascii')

            # a new object has been detected - publish to MQTT
            mqtt_message = {
                "annotated_image": im_b64,
                "raw_image": frame_b64,
                "timestamp": int(time.time())
            }
            mqttc.publish(mqtt_topic, json.dumps(mqtt_message), qos=1)

        message = {
            "image": im_b64,
            "threatcount": interesting_objects.count
        }
        if inference_speed is not None:
            message['inference'] = math.ceil(inference_speed * 100) / 100
        announcer.announce(format_sse(data=json.dumps(message), event="image", retry=retry))

    cam.release()


@app.route("/")
def home():
    return redirect("/index.html")


@app.route("/livez")
@app.route("/readyz")
@app.route("/healthz")
def health():
    return "OK"


def format_sse(data: str, event=None, retry=None) -> str:
    msg = f'data: {data}\n\n'
    if event is not None:
        msg = f'event: {event}\n{msg}'
    if retry is not None:
        msg = f'retry: {retry}\n{msg}'
    return msg

@app.route('/listen', methods=['GET'])
def listen():

    def stream():
        messages = announcer.listen()  # returns a queue.Queue
        while True:
            msg = messages.get()  # blocks until a new message arrives
            yield msg

    return Response(stream(), mimetype='text/event-stream')


if __name__ == '__main__':
    logging.basicConfig(stream=sys.stdout, level=logging.INFO)

    port_str = os.getenv('PORT', '8080')
    port = 0
    try:
        port = int(port_str)
    except ValueError:
        logging.error(f'could not convert PORT ({port_str}) to integer')
        sys.exit(1)

    camera_device = os.getenv('CAMERA', '/dev/video0')

    interested_classes_str = os.getenv('INTERESTED_CLASSES')
    if interested_classes_str is None:
        logging.error('could not get INTERESTED_CLASSES environment variable')
        sys.exit(1)

    interested_classes = []
    for x in interested_classes_str.split(','):
        try:
            i = int(x)
            interested_classes.append(i)
        except ValueError:
            logging.error(f'could not convert INTERESTED_CLASSES component {x} to string')
            sys.exit(1)

    mqtt_topic = os.getenv('MQTT_TOPIC')
    mqtt_server = os.getenv('MQTT_SERVER')
    mqtt_port_str = os.getenv('MQTT_PORT', '1883')
    mqtt_port = None
    try:
        mqtt_port = int(mqtt_port_str)
    except ValueError:
        logging.error(f'could not convert MQTT_PORT ({mqtt_port_str}) to integer')
        sys.exit(1)

    mqttc = None
    if mqtt_topic is None or mqtt_server is None:
        logging.info('starting without mqtt client')
    else:
        mqttc = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, reconnect_on_failure=True)
        mqttc.on_connect = mqtt_on_connect
        mqttc.connect(mqtt_server, mqtt_port, 60)
        mqttc.loop_start()

    force_cpu_lower = os.getenv('FORCE_CPU', 'no').lower()
    force_cpu = False
    if force_cpu_lower == '1' or force_cpu_lower == 'true' or force_cpu_lower == 'yes':
        force_cpu = True

    announcer = MessageAnnouncer()

    with app.app_context():
        continue_running = threading.Event()
        continue_running.set()
        background_thread = threading.Thread(
            target=detection_task,
            args=(camera_device, force_cpu, interested_classes, mqttc, mqtt_topic)
        )
        background_thread.start()

    app.run(host='0.0.0.0', port=port)
    stop_detection_task()
    if mqttc is not None:
        mqttc.loop_stop()
