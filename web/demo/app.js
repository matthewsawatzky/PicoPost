/* PicoPost demo app. */
(function () {
  "use strict";

  var SERVER = "http://127.0.0.1:8080";
  var channel = "chat/general";

  var els = {
    identityStatus: document.getElementById("identity-status"),
    nameForm: document.getElementById("name-form"),
    nameInput: document.getElementById("name-input"),
    resetIdentity: document.getElementById("reset-identity"),
    identityError: document.getElementById("identity-error"),
    channelForm: document.getElementById("channel-form"),
    channelInput: document.getElementById("channel-input"),
    postForm: document.getElementById("post-form"),
    postText: document.getElementById("post-text"),
    metaInput: document.getElementById("meta-input"),
    postError: document.getElementById("post-error"),
    postList: document.getElementById("post-list"),
    streamBadge: document.getElementById("stream-badge"),
  };

  var pp = PicoPost.init({ server: SERVER, channel: channel });

  function showError(el, msg) {
    el.textContent = msg;
    el.hidden = false;
  }

  function clearError(el) {
    el.hidden = true;
  }

  function refreshIdentity() {
    pp.identity.get().then(function (ident) {
      els.identityStatus.textContent =
        "id " + ident.id + (ident.display_name ? " — “" + ident.display_name + "”" : " — no display name yet");
      els.nameInput.value = ident.display_name || "";
      clearError(els.identityError);
    }).catch(function (err) {
      showError(els.identityError, "Identity error: " + err.message);
    });
  }

  function renderPost(post) {
    var li = document.createElement("li");
    var head = document.createElement("div");
    head.className = "post-head";
    var name = document.createElement("span");
    name.className = "post-name";
    name.textContent = post.display_name || "anonymous";
    var time = document.createElement("span");
    time.textContent = new Date(post.created_at * 1000).toLocaleTimeString();
    head.appendChild(name);
    head.appendChild(time);
    li.appendChild(head);

    if (post.meta && Object.keys(post.meta).length) {
      var meta = document.createElement("div");
      meta.className = "post-meta";
      meta.textContent = JSON.stringify(post.meta);
      li.appendChild(meta);
    }

    var text = document.createElement("p");
    text.className = "post-text";
    text.textContent = post.text;
    li.appendChild(text);
    return li;
  }

  function loadPosts() {
    pp.list({ channel: channel, limit: 50 }).then(function (posts) {
      els.postList.innerHTML = "";
      posts.forEach(function (post) {
        els.postList.appendChild(renderPost(post));
      });
    }).catch(function (err) {
      showError(els.postError, "List error: " + err.message);
    });
  }

  pp.subscribe(function (post) {
    els.streamBadge.hidden = false;
    els.postList.prepend(renderPost(post));
  });

  els.nameForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var name = els.nameInput.value.trim();
    pp.identity.setName(name).then(function () {
      refreshIdentity();
    }).catch(function (err) {
      showError(els.identityError, "Name error: " + err.message);
    });
  });

  els.resetIdentity.addEventListener("click", function () {
    pp.identity.reset().then(function () {
      refreshIdentity();
    });
  });

  els.channelForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    channel = els.channelInput.value.trim() || channel;
    pp.init({ server: SERVER, channel: channel });
    loadPosts();
  });

  els.postForm.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var text = els.postText.value.trim();
    if (!text) return;
    var meta = null;
    if (els.metaInput.value.trim()) {
      try {
        meta = JSON.parse(els.metaInput.value);
      } catch (e) {
        showError(els.postError, "Metadata must be valid JSON");
        return;
      }
    }
    pp.send(text, meta).then(function () {
      els.postText.value = "";
      els.metaInput.value = "";
      clearError(els.postError);
    }).catch(function (err) {
      showError(els.postError, "Send error: " + err.message);
    });
  });

  refreshIdentity();
  loadPosts();
})();
