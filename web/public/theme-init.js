// Applies the saved theme before first paint to avoid a light/dark flash.
// Kept as an external file so the Content-Security-Policy can forbid inline scripts.
try {
  var t = localStorage.getItem("pulsewatch.theme");
  if (t === "dark" || t === "light") document.documentElement.setAttribute("data-theme", t);
} catch (e) {}
