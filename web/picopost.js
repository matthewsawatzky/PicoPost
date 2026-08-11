/* PicoPost browser client.
 *
 * Tiny, dependency-free client for the PicoPost API. Usable with a
 * script tag or as a plain module.
 *
 *   <script src="https://posts.example.com/picopost.js" data-channel="comments"></script>
 *
 * or explicitly:
 *
 *   PicoPost.init({ server: "http://localhost:8080", channel: "chat/general" })
 *
 * The client creates a browser identity on first use, stores the secret
 * key in localStorage, restores it on later visits, and authenticates
 * automatically.
 *
 * Multiple independent instances are supported:
 *
 *   var chat = PicoPost.create({ server: "...", channel: "chat/general" });
 *   var comments = PicoPost.create({ server: "...", channel: "comments/article-1" });
 *
 * Each instance has its own identity, stream, and state. The default
 * instance is available as PicoPost.send / PicoPost.list / etc. after
 * PicoPost.init().
 */
(function (global) {
  "use strict";

  var STORAGE_KEY = "picopost.identity";

  function makeInstance(opts) {
    var state = {
      server: null,
      channel: null,
      identity: null, // { id, key }
      es: null,
      callbacks: [],
      ready: false,
    };

    function api(path, options) {
      options = options || {};
      var headers = options.headers || {};
      headers["Content-Type"] = "application/json";
      if (state.identity && state.identity.key) {
        headers["Authorization"] = "Bearer " + state.identity.key;
      }
      return fetch(state.server + path, {
        method: options.method || "GET",
        headers: headers,
        body: options.body ? JSON.stringify(options.body) : undefined,
      }).then(function (res) {
        return res.json().then(function (data) {
          if (!res.ok) {
            var err = new Error((data && data.error) || ("HTTP " + res.status));
            err.status = res.status;
            err.code = data && data.error;
            throw err;
          }
          return data;
        });
      });
    }

    function loadIdentity() {
      try {
        var raw = localStorage.getItem(STORAGE_KEY);
        if (raw) state.identity = JSON.parse(raw);
      } catch (e) {
        state.identity = null;
      }
    }

    function saveIdentity() {
      try {
        if (state.identity) {
          localStorage.setItem(STORAGE_KEY, JSON.stringify(state.identity));
        } else {
          localStorage.removeItem(STORAGE_KEY);
        }
      } catch (e) {
        /* storage unavailable; identity lasts for this page only */
      }
    }

    function ensureIdentity() {
      if (state.identity && state.identity.key) return Promise.resolve(state.identity);
      return api("/v1/identity", { method: "POST" }).then(function (data) {
        state.identity = { id: data.id, key: data.key };
        saveIdentity();
        return state.identity;
      });
    }

    function connect() {
      if (!state.server || !state.channel) return;
      if (state.es) state.es.close();
      var url = state.server + "/v1/stream?channel=" + encodeURIComponent(state.channel);
      var es = new EventSource(url);
      state.es = es;
      es.addEventListener("post", function (ev) {
        var post;
        try {
          post = JSON.parse(ev.data);
        } catch (e) {
          return;
        }
        state.callbacks.forEach(function (cb) {
          try {
            cb(post);
          } catch (e) {
            /* callback errors must not break the stream */
          }
        });
      });
      es.onerror = function () {
        /* EventSource reconnects automatically; nothing to do. */
      };
    }

    return {
      init: function (opts) {
        opts = opts || {};
        state.server = (opts.server || "").replace(/\/+$/, "");
        state.channel = opts.channel || null;
        var script = document.querySelector("script[data-channel]");
        if (!state.server && script) {
          state.server = script.src.replace(/\/picopost\.js.*$/, "");
        }
        if (!state.channel && script) {
          state.channel = script.getAttribute("data-channel");
        }
        if (!state.server) throw new Error("PicoPost.init: server is required");
        loadIdentity();
        state.ready = true;
        connect();
        return this;
      },

      send: function (text, meta) {
        if (!state.ready) return Promise.reject(new Error("PicoPost.init() must be called first"));
        return ensureIdentity().then(function () {
          var body = { channel: state.channel, text: text };
          if (meta) body.meta = meta;
          return api("/v1/posts", { method: "POST", body: body });
        });
      },

      // sendAnon posts without a browser identity. The server must
      // allow anonymous posting ([identity] anonymous = true).
      sendAnon: function (text, meta, displayName) {
        if (!state.ready) return Promise.reject(new Error("PicoPost.init() must be called first"));
        var body = { channel: state.channel, text: text };
        if (meta) body.meta = meta;
        if (displayName) body.display_name = displayName;
        return api("/v1/posts", { method: "POST", body: body });
      },

      list: function (opts) {
        if (!state.ready) return Promise.reject(new Error("PicoPost.init() must be called first"));
        opts = opts || {};
        var q = "?channel=" + encodeURIComponent(opts.channel || state.channel);
        if (opts.limit) q += "&limit=" + opts.limit;
        if (opts.before) q += "&before=" + opts.before;
        return api("/v1/posts" + q).then(function (data) {
          return data.posts;
        });
      },

      subscribe: function (callback) {
        state.callbacks.push(callback);
        return function () {
          state.callbacks = state.callbacks.filter(function (cb) {
            return cb !== callback;
          });
        };
      },

      identity: {
        get: function () {
          if (!state.ready) return Promise.reject(new Error("PicoPost.init() must be called first"));
          return ensureIdentity().then(function () {
            return api("/v1/identity");
          });
        },
        setName: function (name) {
          if (!state.ready) return Promise.reject(new Error("PicoPost.init() must be called first"));
          return ensureIdentity().then(function () {
            return api("/v1/identity", { method: "PATCH", body: { name: name } });
          });
        },
        reset: function () {
          state.identity = null;
          saveIdentity();
          return Promise.resolve();
        },
      },
    };
  }

  var PicoPost = makeInstance();

  // create returns a new independent instance (own identity, stream,
  // and channel). Useful for multiple widgets on one page.
  PicoPost.create = function (opts) {
    var inst = makeInstance();
    return inst.init(opts);
  };

  global.PicoPost = PicoPost;
})(typeof window !== "undefined" ? window : this);
