/* PicoPost public demo: five widgets on one page. */
(function () {
  "use strict";

  var DEFAULT_SERVER = "https://picopost-demo.fly.dev";
  var server = localStorage.getItem("picopost.demo.server") || DEFAULT_SERVER;

  var els = {
    serverInput: document.getElementById("server-input"),
    serverApply: document.getElementById("server-apply"),
    serverError: document.getElementById("server-error"),
    connBadge: document.getElementById("conn-badge"),
    who: document.getElementById("who"),
    nameInput: document.getElementById("name-input"),
    setName: document.getElementById("set-name"),
    resetIdentity: document.getElementById("reset-identity"),
  };

  var widgets = {
    chat: { channel: "chat/general", list: document.getElementById("chat-posts"), form: document.getElementById("chat-form") },
    comments: { channel: "comments/article-1", list: document.getElementById("comments-posts"), form: document.getElementById("comments-form") },
    reviews: { channel: "reviews/homepage", list: document.getElementById("reviews-posts"), form: document.getElementById("reviews-form") },
    agent: { channel: "agent/help", list: document.getElementById("agent-posts"), form: document.getElementById("agent-form") },
    guestbook: { channel: "guestbook/main", list: document.getElementById("guestbook-posts"), form: document.getElementById("guestbook-form") },
  };

  var instances = {};
  var agentReplies = {
    "hello": "Hi there! I'm the PicoPost demo agent. Ask me about pricing, features, or how to install.",
    "pricing": "PicoPost is free and open source (MIT). You host it yourself, so the only cost is your server.",
    "price": "PicoPost is free and open source (MIT). You host it yourself, so the only cost is your server.",
    "install": "Install with one command: curl -fsSL https://raw.githubusercontent.com/matthewsawatzky/PicoPost/main/install.sh | sh",
    "feature": "PicoPost gives you posts, channels, browser identities, SSE live updates, CORS, filters, and rate limiting. No accounts, no database server, no Docker.",
    "database": "SQLite. One file, no separate database server.",
    "docker": "No Docker needed. It's a single Go binary.",
    "host": "Anywhere you can run a binary: a VPS, a Raspberry Pi, Fly.io, Railway, or a container if you like.",
    "help": "Try asking me about pricing, install, features, or hosting.",
  };
  var agentFallback = "I'm just a demo bot, so I know a few things: pricing, install, features, hosting, database. Try one of those!";

  function showError(el, msg) {
    el.textContent = msg;
    el.hidden = false;
  }

  function clearError(el) {
    el.hidden = true;
  }

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function renderPost(post, isAgent) {
    var li = document.createElement("li");
    if (post.meta && post.meta.reply_to) li.className = "reply";

    var head = document.createElement("div");
    head.className = "post-head";
    var name = document.createElement("span");
    name.className = "post-name";
    name.textContent = post.display_name || "anonymous";
    if (isAgent) {
      var tag = document.createElement("span");
      tag.className = "agent-tag";
      tag.textContent = "agent";
      name.appendChild(tag);
    }
    var time = document.createElement("span");
    time.textContent = new Date(post.created_at * 1000).toLocaleTimeString();
    head.appendChild(name);
    head.appendChild(time);
    li.appendChild(head);

    if (post.meta && Object.keys(post.meta).length) {
      var meta = document.createElement("div");
      meta.className = "post-meta";
      if (post.meta.stars) {
        var stars = document.createElement("span");
        stars.className = "stars";
        stars.textContent = "★".repeat(post.meta.stars) + "☆".repeat(5 - post.meta.stars);
        meta.appendChild(stars);
        meta.appendChild(document.createTextNode(" " + JSON.stringify(post.meta)));
      } else {
        meta.textContent = JSON.stringify(post.meta);
      }
      li.appendChild(meta);
    }

    var text = document.createElement("p");
    text.className = "post-text";
    text.textContent = post.text;
    li.appendChild(text);
    return li;
  }

  function loadPosts(widget, inst) {
    inst.list({ channel: widget.channel, limit: 30 }).then(function (posts) {
      widget.list.innerHTML = "";
      if (!posts.length) {
        var empty = document.createElement("div");
        empty.className = "empty";
        empty.textContent = "No posts yet — be the first!";
        widget.list.appendChild(empty);
        return;
      }
      posts.forEach(function (post) {
        widget.list.appendChild(renderPost(post, widget.channel === "agent/help" && post.meta && post.meta.agent));
      });
    }).catch(function (err) {
      showError(widget.form.querySelector(".error"), "Load failed: " + err.message);
    });
  }

  function connectAll() {
    Object.keys(widgets).forEach(function (key) {
      var widget = widgets[key];
      var inst = PicoPost.create({ server: server, channel: widget.channel });
      instances[key] = inst;
      inst.subscribe(function (post) {
        widget.list.prepend(renderPost(post, key === "agent" && post.meta && post.meta.agent));
      });
      loadPosts(widget, inst);
    });
  }

  function refreshIdentity() {
    instances.chat.identity.get().then(function (ident) {
      els.who.innerHTML = "Identity: <strong>" + esc(ident.display_name || "no name yet") + "</strong> <span class=\"small\">(" + esc(ident.id) + ")</span>";
      els.nameInput.value = ident.display_name || "";
    }).catch(function () {
      els.who.textContent = "Identity: unavailable";
    });
  }

  function setServer(url) {
    server = url.replace(/\/+$/, "");
    localStorage.setItem("picopost.demo.server", server);
    els.serverInput.value = server;
    els.connBadge.textContent = "connecting";
    els.connBadge.classList.remove("off");
    clearError(els.serverError);
    connectAll();
    refreshIdentity();
  }

  function agentReply(text) {
    var lower = text.toLowerCase();
    for (var key in agentReplies) {
      if (lower.indexOf(key) !== -1) {
        return agentReplies[key];
      }
    }
    return agentFallback;
  }

  function sendAgentReply(question) {
    var reply = agentReply(question);
    var meta = { agent: true, reply_to: "question" };
    instances.agent.sendAnon(reply, meta, "PicoPost Agent").catch(function (err) {
      showError(widgets.agent.form.querySelector(".error"), "Agent reply failed: " + err.message);
    });
  }

  // --- wiring ---

  els.serverInput.value = server;
  els.serverApply.addEventListener("click", function () {
    setServer(els.serverInput.value.trim());
  });

  els.setName.addEventListener("click", function () {
    var name = els.nameInput.value.trim();
    instances.chat.identity.setName(name).then(function () {
      refreshIdentity();
    }).catch(function (err) {
      showError(els.serverError, "Name error: " + err.message);
    });
  });

  els.resetIdentity.addEventListener("click", function () {
    instances.chat.identity.reset().then(function () {
      refreshIdentity();
    });
  });

  widgets.chat.form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var input = this.querySelector("input");
    var text = input.value.trim();
    if (!text) return;
    instances.chat.send(text).then(function () {
      input.value = "";
      clearError(this.querySelector(".error"));
    }.bind(this)).catch(function (err) {
      showError(this.querySelector(".error"), "Send failed: " + err.message);
    }.bind(this));
  });

  widgets.comments.form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var input = this.querySelector("input");
    var text = input.value.trim();
    if (!text) return;
    var meta = { reply_to: "article-1" };
    instances.comments.send(text, meta).then(function () {
      input.value = "";
      clearError(this.querySelector(".error"));
    }.bind(this)).catch(function (err) {
      showError(this.querySelector(".error"), "Comment failed: " + err.message);
    }.bind(this));
  });

  widgets.reviews.form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var input = this.querySelector("input");
    var stars = parseInt(document.getElementById("reviews-stars").value, 10);
    var text = input.value.trim();
    if (!text) return;
    instances.reviews.send(text, { stars: stars }).then(function () {
      input.value = "";
      clearError(this.querySelector(".error"));
    }.bind(this)).catch(function (err) {
      showError(this.querySelector(".error"), "Review failed: " + err.message);
    }.bind(this));
  });

  widgets.agent.form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var input = this.querySelector("input");
    var text = input.value.trim();
    if (!text) return;
    instances.agent.send(text).then(function () {
      input.value = "";
      clearError(this.querySelector(".error"));
      sendAgentReply(text);
    }).catch(function (err) {
      showError(this.querySelector(".error"), "Ask failed: " + err.message);
    });
  });

  widgets.guestbook.form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    var input = this.querySelector("input");
    var text = input.value.trim();
    if (!text) return;
    instances.guestbook.send(text).then(function () {
      input.value = "";
      clearError(this.querySelector(".error"));
    }.bind(this)).catch(function (err) {
      showError(this.querySelector(".error"), "Sign failed: " + err.message);
    }.bind(this));
  });

  // --- boot ---

  connectAll();
  refreshIdentity();

  // Health check for the connection badge.
  fetch(server + "/health").then(function (res) {
    if (res.ok) {
      els.connBadge.textContent = "connected";
    } else {
      els.connBadge.textContent = "unreachable";
      els.connBadge.classList.add("off");
    }
  }).catch(function () {
    els.connBadge.textContent = "unreachable";
    els.connBadge.classList.add("off");
  });
})();
