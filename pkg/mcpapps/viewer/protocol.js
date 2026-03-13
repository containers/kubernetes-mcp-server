(function() {
  'use strict';

  var nextId = 1;
  var pending = new Map();
  var notificationHandlers = {};
  var requestHandlers = {};

  function sendMessage(msg) {
    window.parent.postMessage(msg, '*');
  }

  function sendRequest(method, params) {
    var id = nextId++;
    sendMessage({ jsonrpc: '2.0', id: id, method: method, params: params });
    return new Promise(function(resolve, reject) {
      pending.set(id, { resolve: resolve, reject: reject });
    });
  }

  function sendNotification(method, params) {
    sendMessage({ jsonrpc: '2.0', method: method, params: params });
  }

  function onNotification(method, handler) {
    notificationHandlers[method] = handler;
  }

  function onRequest(method, handler) {
    requestHandlers[method] = handler;
  }

  window.addEventListener('message', function(event) {
    var msg = event.data;
    if (!msg || msg.jsonrpc !== '2.0') {
      console.log('[mcp-protocol] non-jsonrpc message:', typeof msg === 'object' ? JSON.stringify(msg).substring(0, 200) : msg);
      return;
    }

    console.log('[mcp-protocol] <<', msg.method || ('response id=' + msg.id), JSON.stringify(msg).substring(0, 500));

    // Response to our request
    if (msg.id != null && pending.has(msg.id)) {
      var p = pending.get(msg.id);
      pending.delete(msg.id);
      if (msg.error) p.reject(new Error(msg.error.message));
      else p.resolve(msg.result);
      return;
    }

    // Incoming request (has id + method)
    if (msg.method && msg.id != null) {
      console.log('[mcp-protocol] request:', msg.method, 'id=' + msg.id);
      if (requestHandlers[msg.method]) {
        var result = requestHandlers[msg.method](msg.params);
        sendMessage({ jsonrpc: '2.0', id: msg.id, result: result || {} });
      } else if (msg.method === 'ping') {
        sendMessage({ jsonrpc: '2.0', id: msg.id, result: {} });
      } else {
        console.warn('[mcp-protocol] UNHANDLED request:', msg.method);
        // Respond with empty result to avoid hanging the host
        sendMessage({ jsonrpc: '2.0', id: msg.id, result: {} });
      }
      return;
    }

    // Incoming notification (no id)
    if (msg.method) {
      if (notificationHandlers[msg.method]) {
        console.log('[mcp-protocol] dispatching notification:', msg.method);
        notificationHandlers[msg.method](msg.params);
      } else {
        console.warn('[mcp-protocol] UNHANDLED notification:', msg.method, JSON.stringify(msg.params).substring(0, 300));
      }
    }
  });

  function initialize() {
    return sendRequest('ui/initialize', {
      appInfo: { name: 'kubernetes-mcp-viewer', version: '1.0.0' },
      protocolVersion: '2026-01-26',
      appCapabilities: {},
    }).then(function(result) {
      sendNotification('ui/notifications/initialized', {});
      return result;
    });
  }

  // Expose protocol API
  window.mcpProtocol = {
    initialize: initialize,
    onNotification: onNotification,
    onRequest: onRequest,
    sendRequest: sendRequest,
    sendNotification: sendNotification
  };
})();
