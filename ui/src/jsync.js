function jsync(events) {
  var object = {}
  object.httpsEnabled = window.location.protocol == "https:";
  object.args = window.location.search;
  object.url = (object.httpsEnabled ? 'wss://' : 'ws://') + window.location.host + window.location.pathname + 'sync';
  object.protocols = ['jsync'];
  object.events = events || {}
  object.autoReconnect = 1;

  object.openWs = function () {
    object.ws = new WebSocket(object.url, object.protocols);

    object.ws.onopen = function (event) {
      // object.ws.send("hello")
      for (var k in object.events) {
        object.action('watch', k, {})
      }
    }

    object.ws.onmessage = function (event) {
      var data = JSON.parse(event.data);
      var callback = object.events[data.name] || function () {}
      callback(data.name, data.data)
      // object.callback(JSON.parse(data));
      console.log(data);
    }

    object.ws.onclose = function (event) {
      if (object.autoReconnect > 0) {
        setTimeout(object.openWs, object.autoReconnect * 1000)
      }
    }
  }
  object.action = function (action, name, data) {
    var comand = {
      action: action,
      name: name,
      data: data
    }
    object.ws.send(JSON.stringify(comand))

  }

  object.sendPing = function (ws) {
    ws.send("1")
  }

  object.openWs()
  return object;
}
var myjsync = { jsync: jsync }

export default myjsync