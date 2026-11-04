/*
 * Widget bootstrap for Element.
 *
 * Element does not put the user's OpenID token in the widget URL. The widget
 * asks for it through the Matrix Widget API (postMessage, action "get_openid"),
 * then sends it in the X-OpenID-Token header of every request. The token is
 * renewed shortly before it expires, and never appears in a URL.
 *
 * Outside Element (direct browsing, development) there is no parent frame to
 * ask. The page then relies on the "openIdToken" or "userId" query parameters,
 * which are forwarded as headers.
 */
(function () {
    'use strict';

    var params = new URLSearchParams(window.location.search);
    var roomId = params.get('roomId') || '';
    var devUser = params.get('userId') || '';
    var widgetId = params.get('widgetId') || '';
    var inFrame = window.parent !== window;

    var REFRESH_MARGIN_MS = 120000;
    var FIRST_ANSWER_TIMEOUT_MS = 5000;
    var USER_CONFIRM_TIMEOUT_MS = 300000;
    var RETRY_AFTER_FAILURE_MS = 60000;

    var credentials = { token: params.get('openIdToken') || '', expiresAt: Infinity };
    var lastFailure = 0;
    var inFlight = null;
    var pending = {};
    var waitingForWidgetId = [];

    function post(message) {
        window.parent.postMessage(message, '*');
    }

    function onWidgetId(callback) {
        if (widgetId) {
            callback();
            return;
        }
        waitingForWidgetId.push(callback);
    }

    function learnWidgetId(id) {
        widgetId = id;
        var callbacks = waitingForWidgetId;
        waitingForWidgetId = [];
        callbacks.forEach(function (callback) { callback(); });
    }

    function settle(requestId, data) {
        if (pending[requestId]) {
            pending[requestId](data || {});
        }
    }

    // The client sends requests such as "capabilities". They must be answered
    // or Element reports the widget as unresponsive. This widget needs none.
    function answerClient(request) {
        var response = {};
        if (request.action === 'capabilities') {
            response = { capabilities: [] };
        } else if (request.action === 'supported_api_versions') {
            response = { supported_versions: ['0.0.1', '0.0.2', '1.0.0', '1.1.0'] };
        }
        var reply = {};
        Object.keys(request).forEach(function (key) { reply[key] = request[key]; });
        reply.response = response;
        post(reply);
    }

    function onMessage(event) {
        if (event.source !== window.parent) { return; }
        var message = event.data;
        if (!message || !message.action || !message.requestId || !message.widgetId) { return; }

        if (message.api === 'toWidget' && !message.response) {
            if (!widgetId) { learnWidgetId(message.widgetId); }
            if (message.widgetId !== widgetId) { return; }
            if (message.action === 'openid_credentials') {
                settle(message.data && message.data.original_request_id, message.data);
            }
            answerClient(message);
        } else if (message.api === 'fromWidget' && message.response) {
            if (message.widgetId !== widgetId) { return; }
            settle(message.requestId, message.response);
        }
    }

    // requestOpenId resolves with {token, expiresAt}, or null when the client
    // does not answer, or the user does not allow it.
    function requestOpenId() {
        if (!inFrame) { return Promise.resolve(null); }
        if (inFlight) { return inFlight; }

        inFlight = new Promise(function (resolve) {
            var id = 'frequencia-' + Date.now() + '-' + Math.random().toString(36).slice(2);
            var timer = setTimeout(function () { finish(null); }, FIRST_ANSWER_TIMEOUT_MS);

            function finish(result) {
                clearTimeout(timer);
                delete pending[id];
                inFlight = null;
                resolve(result);
            }

            pending[id] = function (data) {
                if (data.state === 'request') {
                    // Element is asking the user to allow it: wait for the answer.
                    clearTimeout(timer);
                    timer = setTimeout(function () { finish(null); }, USER_CONFIRM_TIMEOUT_MS);
                    return;
                }
                if (data.state === 'allowed' && data.access_token) {
                    finish({
                        token: data.access_token,
                        expiresAt: Date.now() + (data.expires_in || 3600) * 1000
                    });
                    return;
                }
                finish(null);
            };

            onWidgetId(function () {
                post({ api: 'fromWidget', widgetId: widgetId, requestId: id, action: 'get_openid', data: {} });
            });
        });
        return inFlight;
    }

    function tokenIsFresh() {
        if (!inFrame) { return true; }
        return credentials.token !== '' && Date.now() < credentials.expiresAt - REFRESH_MARGIN_MS;
    }

    function ensureToken() {
        if (tokenIsFresh()) { return Promise.resolve(); }
        if (lastFailure && Date.now() - lastFailure < RETRY_AFTER_FAILURE_MS) { return Promise.resolve(); }
        return requestOpenId().then(function (fresh) {
            if (fresh) {
                credentials = fresh;
                lastFailure = 0;
            } else {
                lastFailure = Date.now();
            }
        });
    }

    function authHeaders() {
        var headers = {};
        if (credentials.token) { headers['X-OpenID-Token'] = credentials.token; }
        if (devUser) { headers['X-Dev-User'] = devUser; }
        return headers;
    }

    function download(url, filename) {
        return ensureToken()
            .then(function () { return fetch(url, { headers: authHeaders() }); })
            .then(function (response) {
                if (!response.ok) { throw new Error('download failed with status ' + response.status); }
                return response.blob();
            })
            .then(function (blob) {
                var link = document.createElement('a');
                link.href = URL.createObjectURL(blob);
                link.download = filename;
                document.body.appendChild(link);
                link.click();
                link.remove();
                setTimeout(function () { URL.revokeObjectURL(link.href); }, 10000);
            })
            .catch(function (error) { console.error(error); });
    }

    function highlightTab(source) {
        var tabs = document.getElementById('tabs');
        if (!tabs || !source || source.parentElement !== tabs) { return; }
        tabs.querySelectorAll('button').forEach(function (button) {
            var active = button === source;
            button.classList.toggle('border-indigo-600', active);
            button.classList.toggle('text-indigo-700', active);
            button.classList.toggle('border-transparent', !active);
        });
    }

    function boot() {
        var app = document.getElementById('app');
        if (!app || app.getAttribute('data-shell') !== 'true') { return; }
        ensureToken().then(function () {
            window.htmx.ajax('GET', '/widget/home?roomId=' + encodeURIComponent(roomId), { target: '#app', swap: 'innerHTML' });
        });
    }

    window.addEventListener('message', onMessage);

    document.addEventListener('htmx:configRequest', function (event) {
        var headers = authHeaders();
        Object.keys(headers).forEach(function (name) { event.detail.headers[name] = headers[name]; });
    });

    // Delay a request while the token is missing or about to expire.
    document.addEventListener('htmx:confirm', function (event) {
        if (tokenIsFresh()) { return; }
        event.preventDefault();
        ensureToken().then(function () { event.detail.issueRequest(false); });
    });

    // Show the "restricted access" card, which comes with status 403.
    document.addEventListener('htmx:beforeSwap', function (event) {
        if (event.detail.xhr.status === 403) {
            event.detail.shouldSwap = true;
            event.detail.isError = false;
        }
    });

    document.addEventListener('htmx:beforeRequest', function (event) {
        highlightTab(event.detail.elt);
    });

    document.addEventListener('click', function (event) {
        var button = event.target.closest('[data-download]');
        if (!button) { return; }
        event.preventDefault();
        download(button.getAttribute('data-download'), button.getAttribute('data-filename') || 'download');
    });

    boot();
}());
