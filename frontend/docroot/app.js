var photo = null;
var rawImage = null;
var annotatedImage = null;
var showAnnotated = null;
var llmResponse = null;
var llmResponseSpinner = null;
var prompt = null;
var promptChoices = null;
var promptChoicesSpinner = null;
var sseErrors = 0;

function loadPromptChoices() {
  // clear out existing choices
  promptChoices.innerHTML = '';
  promptChoices.style.display = 'none';
  promptChoicesSpinner.style.display = 'block';
  // todo: fire off REST call to /api/prompt
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
      let d = document.createElement('div');
      d.className = 'prompt-choice';
      d.innerText = p;
      promptChoices.appendChild(d);
    })
  })
}
function setPrompt(event) {
  prompt.innerText = event.data;
}

function showLLMResponseSpinner(event) {
  llmResponse.value = '';
  llmResponse.style.display = 'none';
  llmResponseSpinner.style.display = 'block';
}

function hideLLMResponseSpinner(event) {
  llmResponseSpinner.style.display = 'none';
  llmResponse.style.display = 'block';
}

function processLLMResponse(event) {
  if (event == null || event.data == null) return;
  const obj = JSON.parse(event.data);
  if (obj == null || obj.response == null) return;
  llmResponse.value += obj.response;
}

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
  llmResponse = document.getElementById('llm-response');
  llmResponseSpinner = document.getElementById('llm-response-spinner');
  prompt = document.getElementById('prompt');
  promptChoices = document.getElementById('prompt-choices');
  promptChoicesSpinner = document.getElementById('prompt-choices-spinner');

  const evtSource = new EventSource("/api/sse");
  evtSource.addEventListener("annotated_image", processImageEvent);
  evtSource.addEventListener("raw_image", processImageEvent);
  evtSource.addEventListener("llm_request_start", showLLMResponseSpinner);
  evtSource.addEventListener("llm_response", processLLMResponse);
  evtSource.addEventListener("llm_response_start", hideLLMResponseSpinner);
  evtSource.addEventListener("prompt", setPrompt);

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