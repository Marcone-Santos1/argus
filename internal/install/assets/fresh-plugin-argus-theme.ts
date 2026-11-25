// Argus: the editor follows the theme chosen in Argus (key T), live.
// Argus writes the theme's name to argus-theme next to config.json.
const argusThemeFile = editor.getConfigDir() + "/argus-theme";
let argusThemeLast = "";

registerHandler("argus_theme_check", function () {
  try {
    const name = (editor.readFile(argusThemeFile) || "").trim();
    if (name !== "" && name !== argusThemeLast) {
      argusThemeLast = name;
      editor.reloadThemes();
      editor.applyTheme(name);
    }
  } catch (e) {
    editor.warn("argus-theme: " + e);
  }
});

editor.on("path_changed", "argus_theme_check");
editor.watchPath(editor.getConfigDir()).catch(function () {
  editor.setInterval(1000, "argus_theme_check"); // no watch: look every second
});
argus_theme_check();
