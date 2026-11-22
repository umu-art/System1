package main

import "html/template"

var folderPicker = template.Must(template.New("folderPicker").Parse(`<!doctype html>
<html lang="ru">
<meta charset="utf-8">
<title>Выбор папки System1</title>
<p id="status">Выберите папку System1, в которую будут выгружаться расшифровки.</p>
<button id="select" disabled>Выбрать папку</button>
<script>
function googlePickerLoaded() {
  gapi.load('picker', function() {
    const button = document.getElementById('select');
    button.disabled = false;
    button.onclick = function() {
      const view = new google.picker.DocsView(google.picker.ViewId.FOLDERS);
      view.setIncludeFolders(true).setSelectFolderEnabled(true);
      new google.picker.PickerBuilder()
        .addView(view)
        .setAppId({{.Project}})
        .setDeveloperKey({{.Key}})
        .setOAuthToken({{.AccessToken}})
        .setOrigin(location.origin)
        .setCallback(async function(data) {
          if (data.action !== google.picker.Action.PICKED) return;
          const response = await fetch('/folder', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({id: data.docs[0].id, state: {{.State}}})
          });
          document.getElementById('status').textContent = response.ok
            ? 'Папка подключена. Результат записи секрета будет в консоли.'
            : await response.text();
        })
        .build().setVisible(true);
    };
  });
}
</script>
<script async src="https://apis.google.com/js/api.js" onload="googlePickerLoaded()"></script>
</html>`))
