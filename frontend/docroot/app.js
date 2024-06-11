var timestamp = null;
var photo = null;
var rawImage = null;
var annotatedImage = null;
var showAnnotated = null;
var ollamaResponse = null;
var ollamaResponseSpinner = null;
var openaiResponse = null;
var openaiResponseSpinner = null;
var prompt = null;
var promptChoices = null;
var promptChoicesSpinner = null;
var sseErrors = 0;
var playSound = null;
var sound = new Audio("warning.mp3");
var currentImageTimestamp = 0; // used by the sound.play() logic to determine if we have already played the emergency sound on this image
var resumeButton = null;

// https://www.w3schools.com/howto/howto_js_snackbar.asp
function showMessage(msg) {
  var x = document.getElementById("snackbar");

  x.innerText = msg;

  // Add the "show" class to DIV
  x.className = "show";

  // After 3 seconds, remove the show class from DIV
  setTimeout(function(){
    x.className = x.className.replace("show", "");
  }, 3000);
}

function setNewPromptOnServer(id) {
  showMessage('setting prompt');
  fetch('/api/prompt', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ id: id })
  })
  .then(response => {
    if (response.status >= 200 && response.status < 300) {
      showMessage('prompt set successfully');
      return null;
    }

    showMessage('error setting prompt: ' + response.status);
    return response.text();
  })
  .then(text => {
    if (text == null) return;
    showMessage(text);
  })
  .catch(error => {
    showMessage(error);
  });
}

function loadPromptChoices() {
  // clear out existing choices
  promptChoices.innerHTML = '';
  promptChoices.style.display = 'none';
  promptChoicesSpinner.style.display = 'block';
  fetch('/api/prompt', {
    method: 'GET',
    headers: {
        'Accept': 'application/json',
    },
  })
  .then(response => response.json())
  .then(response => {
    promptChoicesSpinner.style.display = 'none';
    promptChoices.style.display = 'block';
    if (response == null) return;
    response.forEach(p => {
      if (p.id == null || p.prompt == null) {
        console.log('did not receive expected prompt fields');
        console.log(p);
        return;
      }
      let d = document.createElement('button');
      d.className = 'prompt-choice';
      d.innerText = p.prompt;
      d.onclick = function() { setNewPromptOnServer(p.id) };
      promptChoices.appendChild(d);
    });
    loadCurrentState();
  })
  .catch(error => {
    showMessage(error);
  })
}

function setPrompt(data) {
  prompt.innerText = data;
}

function processPromptEvent(event) {
  if (event == null || event.data == null) return;
  const obj = JSON.parse(event.data);
  if (obj.prompt == null) return;
  setPrompt(obj.prompt);
}

function loadCurrentState() {
  fetch('/api/currentstate', {
    method: 'GET',
    headers: {
        'Accept': 'application/json',
    },
  })
  .then(response => response.json())
  .then(response => {
    if (response == null) return;
    if (response.annotated_image != null) annotatedImage = response.annotated_image;
    if (response.raw_image != null) rawImage = response.raw_image;
    if (response.annotated_image != null || response.raw_image != null) refreshPhoto();
    if (response.timestamp != null) setTimestamp(response.timestamp);
    if (response.prompt != null) setPrompt(response.prompt);
    if (response.image_analysis != null) ollamaResponse.value = response.image_analysis;
    if (response.threat_analysis != null) openaiResponse.value = response.threat_analysis;
    if (response.events_paused != null) response.events_paused?showResumeButton():hideResumeButton();
  })
  .catch(error => {console.log(error);showMessage(error);});
}

function showOllamaResponseSpinner(event) {
  ollamaResponse.value = '';
  ollamaResponse.style.display = 'none';
  ollamaResponseSpinner.style.display = 'block';
}

function hideOllamaResponseSpinner(event) {
  ollamaResponseSpinner.style.display = 'none';
  ollamaResponse.style.display = 'block';
}

function processOllamaResponse(event) {
  if (event == null || event.data == null) return;
  let obj = null;
  try {
    obj = JSON.parse(event.data);
  } catch (e) {
    console.log(e);
    console.log(event);
  }
  if (obj == null || obj.response == null) return;
  ollamaResponse.value += obj.response;
}

function showOpenaiResponseSpinner(event) {
  openaiResponse.value = '';
  openaiResponse.style.display = 'none';
  openaiResponseSpinner.style.display = 'block';
}

function hideOpenaiResponseSpinner(event) {
  openaiResponseSpinner.style.display = 'none';
  openaiResponse.style.display = 'block';
}

function processOpenaiResponse(event) {
  if (event == null || event.data == null) return;
  let obj = null;
  try {
    obj = JSON.parse(event.data);
  } catch (e) {
    console.log(e);
    console.log(event);
  }
  if (obj == null || obj.response == null) return;
  openaiResponse.value += obj.response;
}

function refreshPhoto() {
  let data = (showAnnotated.checked?annotatedImage:rawImage);

  if (data == null) {
    clearPhoto();
    return;
  }
  photo.setAttribute('src', 'data:image/jpeg;charset=utf-8;base64,' + data);
}

function setTimestamp(data) {
  let date = new Date(data * 1000);
  let time = date.toString().split(' ')[4];
  timestamp.innerText = time;
}

function processTimestampEvent(event) {
  if (event == null || event.data == null) return;

  setTimestamp(event.data);

  // play sound if we have never seen this timestamp before
  if (playSound.checked && currentImageTimestamp != event.data) {
    console.log("currentImageTimestamp=" + currentImageTimestamp + " event.data=" + event.data);
    currentImageTimestamp = event.data;
    sound.play();
  }

  showOllamaResponseSpinner();
  showOpenaiResponseSpinner();
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

function resumeEvents() {
  fetch('/api/resumeevents');
}

function showResumeButton() {
  resumeButton.style.display = 'block';
}

function hideResumeButton() {
  resumeButton.style.display = 'none';
}

function startup() {
  timestamp = document.getElementById('timestamp');
  photo = document.getElementById('photo');
  clearPhoto();

  showAnnotated = document.getElementById('show-annotated');
  ollamaResponse = document.getElementById('ollama-response');
  ollamaResponseSpinner = document.getElementById('ollama-response-spinner');
  openaiResponse = document.getElementById('openai-response');
  openaiResponseSpinner = document.getElementById('openai-response-spinner');
  prompt = document.getElementById('prompt');
  promptChoices = document.getElementById('prompt-choices');
  promptChoicesSpinner = document.getElementById('prompt-choices-spinner');
  playSound = document.getElementById('play-sound');
  resumeButton = document.getElementById('resume');

  const evtSource = new EventSource("/api/sse");
  evtSource.addEventListener("timestamp", processTimestampEvent);
  evtSource.addEventListener("annotated_image", processImageEvent);
  evtSource.addEventListener("raw_image", processImageEvent);
  evtSource.addEventListener("ollama_response", processOllamaResponse);
  evtSource.addEventListener("ollama_response_start", hideOllamaResponseSpinner);
  evtSource.addEventListener("openai_response", processOpenaiResponse);
  evtSource.addEventListener("openai_response_start", hideOpenaiResponseSpinner);
  evtSource.addEventListener("prompt", processPromptEvent);
  evtSource.addEventListener("pause_events", showResumeButton);
  evtSource.addEventListener("resume_events", hideResumeButton);

  evtSource.onerror = (e) => {
    sseErrors++;
    if (sseErrors > 50) {
      evtSource.close();
      alert("connection error threshold exceeded, terminating SSE event source");
    }
  };

  loadPromptChoices();
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