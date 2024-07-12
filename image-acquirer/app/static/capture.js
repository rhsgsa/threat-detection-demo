(function() {
    var photo = null;
    var defects = null;
    var sseErrors = 0;
    var inference = null;
    var threatsDiv = null;
    var dropped = null;
  
    function startup() {
      photo = document.getElementById('photo');
      threatcount = document.getElementById('threatcount');
      inference = document.getElementById('inference');
      threatsDiv = document.getElementById('threatsdiv');
      dropped = document.getElementById('dropped');
      clearphoto();

      const evtSource = new EventSource("/listen");
      evtSource.addEventListener("image", (event) => {
        sseErrors = 0;
        if (event == null || event.data == null) return;
        var body = JSON.parse(event.data);
        if (body != null) {
          if (body.image != null) {
            photo.setAttribute('src', 'data:image/jpeg;charset=utf-8;base64,' + body.image);
          }
          let threatsInt = NaN;
          if (body.threatcount != null)
            threatsInt = parseInt(body.threatcount);
          if (isNaN(threatsInt))
            threatsDiv.style.display = 'none';
          else {
            threatsDiv.style.display = 'block';
            threatcount.innerText = body.threatcount;
          }
          if (body.dropped != null)
            dropped.innerText = body.dropped;
          if (body.inference != null)
            inference.innerText = body.inference + ' ms';
          else
          inference.innerText = 'unknown';
        }
      });
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
  
    function clearphoto() {
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
  })();