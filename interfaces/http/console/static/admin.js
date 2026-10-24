// Admin console of the sync server. Every page works without this file: it
// only copies an invitation code to the clipboard.
"use strict";

document.addEventListener("click", (event) => {
  const button = event.target.closest("[data-copy]");
  if (!button || !navigator.clipboard) {
    return;
  }
  const source = document.getElementById(button.dataset.copy);
  if (!source) {
    return;
  }
  navigator.clipboard.writeText(source.textContent.trim()).then(() => {
    const label = button.textContent;
    button.textContent = "Copied";
    setTimeout(() => {
      button.textContent = label;
    }, 1500);
  });
});
