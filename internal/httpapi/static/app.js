(function () {
  var provider = document.querySelector("#provider");
  var selector = document.querySelector("#selector-field");
  function syncSelector() {
    if (!provider || !selector) return;
    selector.hidden = provider.value !== "article_index";
  }
  if (provider) provider.addEventListener("change", syncSelector);
  syncSelector();

  var live = document.querySelector("#live");
  if (!live) return;
  var last = live.innerHTML;
  window.setInterval(async function () {
    try {
      var response = await fetch("/fragment/live", { cache: "no-store" });
      if (!response.ok) return;
      var html = await response.text();
      if (html !== last) {
        last = html;
        live.innerHTML = html;
      }
    } catch (err) {
      /* Leave the last rendered captures in place. */
    }
  }, 2000);
})();
