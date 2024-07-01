import base64
import logging
import sys
import os
import threading
import json
import time
import math
from typing import List

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
    count = 0

    # returns True if a new object is detected
    def update(self, boxes) -> bool:
        if (boxes.id is None) or (boxes.cls is None):
            return False

        cls = boxes.cls.numpy().astype(int)
        ids = boxes.id.numpy().astype(int)

        for _, id in enumerate(ids):
            if self.check_if_tracking_id_is_new(id):
                return True
        return False

    # returns True if id is new
    def check_if_tracking_id_is_new(self, id) -> bool:
        if id in self.object_ids:
            return False
        self.count += 1
        logging.info(f"object id = {id}")
        self.object_ids.append(id)
        if len(self.object_ids) > self.max_objects:
            self.object_ids.pop(0)
        return True


app = Flask(__name__, static_url_path='')

def stop_detection_task():
    logging.info("notifying background thread")
    continue_running.clear()
    background_thread.join()
    logging.info("background thread exited cleanly")

def process_resize_str(resize: str) -> (int, int):
    if resize is None:
        return 0, 0
    parts = resize.split('x')
    if len(parts) < 2:
        return 0, 0
    try:
        return int(parts[0]), int(parts[1])
    except ValueError:
        return 0, 0

def detection_task(camera_device, resize, model_name, confidence, force_cpu, interested_classes, mqttc, mqtt_topic, tracking):
    retry = 500

    if model_name.endswith('.engine'):
        tracking = False
        logging.info('tracking off because we are using TensorRT')

    if tracking:
        interesting_objects = InterestingObjects()
        task = 'track'
        logging.info("tracking on")
    else:
        interesting_objects = None
        task = 'detect'
        logging.info("tracking off")

    resize_width, resize_height = process_resize_str(resize)
    if resize_width > 0 and resize_height > 0:
        logging.info(f"will resize input images to {resize_width}x{resize_height}")
    else:
        logging.info("will not resize images")

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

    logging.info(f"loading model ({model_name})...")
    model = YOLO(model_name, task=task)
    logging.info("done loading model")
    try:
        if hasattr(model, 'names'):
            logging.info(f"model names = {model.names}")
    except:
        pass

    torch.device(accel_device)
    if accel_device != "cpu" and model_name.endswith('.pt'):
        logging.info(f"moving model to {accel_device}")
        model.to(accel_device)

    cam = cv2.VideoCapture(camera_device)
    retry_pause = False

    while continue_running.is_set():
        result, cam_frame = cam.read()
        if not result:
            logging.info("Video source did not return a frame")
            cam.open(camera_device)
            if retry_pause:
                time.sleep(2)
            else:
                retry_pause = True
            continue

        retry_pause = False

        frame = cam_frame
        if resize_width > 0 and resize_height > 0:
            frame = cv2.resize(cam_frame, (resize_width, resize_height))

        if tracking:
            results = model.track(frame, device=accel_device, conf=confidence, classes=interested_classes)
        else:
            results = model(frame, device=accel_device, conf=confidence, classes=interested_classes)

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

        trigger_event = False
        if tracking:
            if interesting_objects.update(result.boxes):
                trigger_event = True
        else:
            if result.boxes.cls is not None and len(result.boxes.cls) > 0:
                trigger_event = True

        if trigger_event and mqttc is not None:
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
            "image": im_b64
        }
        if interesting_objects is None:
            message['threatcount'] = "unknown"
        else:
            message['threatcount'] = interesting_objects.count
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

# returns None if input is empty
def convert_to_int_list(s: str) -> List[int]:
    if s is None:
        return None
    s = s.strip()
    if len(s) == 0:
        return None
    output = [int(x) for x in s.split(',')]
    if len(output) == 0:
        return None
    return output

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

    try:
        interested_classes = convert_to_int_list(os.getenv('INTERESTED_CLASSES'))
    except ValueError as e:
        logging.error(f'could not convert INTERESTED_CLASSES to int: {e}')
        sys.exit(1)

    mqtt_topic = os.getenv('MQTT_TOPIC', 'alerts')
    mqtt_server = os.getenv('MQTT_SERVER', 'localhost')
    mqtt_transport = os.getenv('MQTT_TRANSPORT', 'tcp')
    mqtt_port_str = os.getenv('MQTT_PORT', '1883')
    mqtt_port = None
    try:
        mqtt_port = int(mqtt_port_str)
    except ValueError:
        logging.error(f'could not convert MQTT_PORT ({mqtt_port_str}) to integer')
        sys.exit(1)

    mqttc = None
    if mqtt_topic is None or mqtt_server is None or mqtt_topic == "" or mqtt_server == "":
        logging.info('starting without mqtt client')
    else:
        mqttc = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, transport=mqtt_transport, reconnect_on_failure=True)
        mqttc.on_connect = mqtt_on_connect
        mqttc.connect(mqtt_server, mqtt_port, 60)
        mqttc.loop_start()

    force_cpu_lower = os.getenv('FORCE_CPU', 'no').lower()
    force_cpu = False
    if force_cpu_lower == '1' or force_cpu_lower == 'true' or force_cpu_lower == 'yes':
        force_cpu = True

    model_name = os.getenv('MODEL', 'best.pt')

    confidence_str = os.getenv('CONFIDENCE', '0.25')
    confidence = 0
    try:
        confidence = float(confidence_str)
    except ValueError:
        logging.error(f'could not convert CONFIDENCE ({confidence_str}) to float')
        sys.exit(1)

    resize = os.getenv('RESIZE', '')

    tracking_lower = os.getenv('TRACKING', 'yes').lower()
    tracking = True
    if tracking_lower == '0' or tracking_lower == 'false' or tracking_lower == 'no':
        tracking = False

    announcer = MessageAnnouncer()

    with app.app_context():
        continue_running = threading.Event()
        continue_running.set()
        background_thread = threading.Thread(
            target=detection_task,
            args=(camera_device, resize, model_name, confidence, force_cpu, interested_classes, mqttc, mqtt_topic, tracking)
        )
        background_thread.start()

    try:
        app.run(host='0.0.0.0', port=port)
    except:
        logging.error("error starting web server")
        sys.exit(1)

    # fall through to here if we received a signal
    stop_detection_task()
    if mqttc is not None:
        mqttc.disconnect()
        mqttc.loop_stop()
