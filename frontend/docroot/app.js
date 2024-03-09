var photo = null;
var rawImage = null;
var annotatedImage = null;
var showAnnotated = null;
var defects = null;
var sseErrors = 0;

function refreshPhoto() {
  let data = (showAnnotated.checked?annotatedImage:rawImage);

  if (data == null) {
    clearPhoto();
    return;
  }
  photo.setAttribute('src', 'data:image/jpeg;charset=utf-8;base64,' + data);
}

function processImageEvent(event) {
  sseErrors = 0;
  if (event == null || event.data == null || event.type == null) return;
  if (event.type == "annotated_image")
    annotatedImage = event.data;
  else
    rawImage = event.data;
  refreshPhoto();
}

function startup() {
  photo = document.getElementById('photo');
  clearPhoto();

  showAnnotated = document.getElementById('show-annotated');

  const evtSource = new EventSource("/api/sse");
  evtSource.addEventListener("annotated_image", processImageEvent);
  evtSource.addEventListener("raw_image", processImageEvent);
  evtSource.onerror = (e) => {
    sseErrors++;
    if (sseErrors > 50) {
      evtSource.close();
      alert("connection error threshold exceeded, terminating SSE event source");
    }
  };
}

// Fill the photo with an indication that none has been
// captured.

function clearPhoto() {
  var canvas = document.createElement('canvas');
  var context = canvas.getContext('2d');
  context.fillStyle = "#AAA";
  context.fillRect(0, 0, canvas.width, canvas.height);

  var data = canvas.toDataURL('image/png');
  photo.setAttribute('src', data);
}

// https://stackoverflow.com/a/46182044
function getJpegBytes(canvas, callback) {
  var fileReader = new FileReader();
  fileReader.addEventListener('loadend', function () {
    callback(this.error, this.result)
  })

  canvas.toBlob(fileReader.readAsArrayBuffer.bind(fileReader), 'image/jpeg')
}

// Set up our event listener to run the startup process
// once loading is complete.
window.addEventListener('load', startup, false);