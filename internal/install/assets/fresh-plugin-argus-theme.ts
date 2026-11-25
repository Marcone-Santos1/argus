// Argus: the editor follows the theme chosen in Argus (key T), live.
//
// Argus writes the theme's name to argus-theme, next to config.json, and
// this reads that small file in a loop: every 250 ms, and every 30 ms while
// Argus's theme window is open (the file's second line says "live"), so
// moving through the list repaints the editor at once. (A file watch was
// tried first: Fresh reports changes a second or two late.)
const argusThemeFile = editor.getConfigDir() + "/argus-theme";
const argusThemeLoad = {}; // this load's identity: a reload ends the old loop
globalThis.__argusThemeLoad = argusThemeLoad;

(async function () {
  let last = "";
  while (globalThis.__argusThemeLoad === argusThemeLoad) {
    let live = false;
    try {
      const lines = (editor.readFile(argusThemeFile) || "").split("\n");
      const name = lines[0].trim();
      live = (lines[1] || "").trim() === "live";
      if (name !== "" && name !== last) {
        last = name;
        editor.reloadThemes();
        editor.applyTheme(name);
      }
    } catch (e) {
      editor.warn("argus-theme: " + e);
    }
    await editor.delay(live ? 30 : 250);
  }
})().catch(function (e) {
  editor.warn("argus-theme: " + e);
});
