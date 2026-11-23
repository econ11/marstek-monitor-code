# macOS: підпис, нотаризація і тест оновлення

Збирає Mac-реліз `scripts/macos-package.sh <версія> [<ідентичність>]`: universal
`.app` (arm64 + x86_64) зі штампом версії, `bin/marstek-monitor-<версія>-macos.dmg`
і `.sha256`. З ідентичністю `"Developer ID Application: <name> (<TEAMID>)"`
програма й dmg підписуються з hardened runtime і міткою часу; коли задано
`NOTARY_KEY_ID`, `NOTARY_ISSUER` і `NOTARY_KEY_PATH`, обидва нотаризуються й
степляться. Без ідентичності все підписано ad-hoc — така програма не може
оновити сама себе (немає Team ID), їй пропонується лише «Завантажити».

## Тест оновлення перед релізом

Перевіряє справжнє оновлення одним кліком на Mac: завантаження dmg, перевірку
SHA-256, підпис і Team ID, заміну бандла й перезапуск. Потрібен сертифікат
Developer ID у зв'язці ключів (з ad-hoc підписом `canSelfUpdate` за задумом
каже «ні»).

Безпека тесту:
- обидві програми працюють лише з `-simulate-quiet` і тимчасовою текою даних:
  встановлена копія, її тека даних і справжня станція не зачіпаються;
- збірки робляться з `UPDATE_TEST=1`: версія є, але **без тегу `release`**.
  Тільки в такій збірці `-simulate*` дозволяє перевірку й встановлення, і
  лише коли `MARSTEK_UPDATE_BASE` — loopback-адреса `http://127.0.0.1:<порт>/`.
  Релізна збірка ігнорує ці змінні завжди. `UPDATE_TEST` ніколи не ставиться
  для релізу;
- після заміни нова програма відкривається з тими самими прапорцями
  `-simulate*`, тож теж працює в тимчасовій теці.

Кроки (ідентичність — ваша, у формі `"Developer ID Application: <name> (<TEAMID>)"`):

1. Зібрати першу версію і покласти її в тестову теку «Програми»:

   ```sh
   UPDATE_TEST=1 scripts/macos-package.sh 1.1.0-test1 "Developer ID Application: <name> (<TEAMID>)"
   mkdir -p ~/Applications/MarstekTest
   M=$(hdiutil attach -nobrowse -readonly bin/marstek-monitor-1.1.0-test1-macos.dmg | awk -F'\t' '/\/Volumes\//{print $NF}')
   ditto "$M/Marstek Monitor.app" ~/Applications/MarstekTest/"Marstek Monitor.app"
   hdiutil detach "$M"
   ```

2. Зібрати другу версію й підготувати теку локального сервера:

   ```sh
   UPDATE_TEST=1 scripts/macos-package.sh 1.1.0-test2 "Developer ID Application: <name> (<TEAMID>)"
   SRV=$(mktemp -d)
   mkdir -p "$SRV/download/v1.1.0-test2"
   cp bin/marstek-monitor-1.1.0-test2-macos.dmg bin/marstek-monitor-1.1.0-test2-macos.dmg.sha256 "$SRV/download/v1.1.0-test2/"
   ```

   і `$SRV/latest.json` у формі відповіді GitHub `releases/latest`:

   ```json
   {
     "tag_name": "v1.1.0-test2",
     "html_url": "http://127.0.0.1:8000/tag/v1.1.0-test2",
     "draft": false,
     "prerelease": false,
     "body": "local update test",
     "assets": [
       {"name": "marstek-monitor-1.1.0-test2-macos.dmg",
        "browser_download_url": "http://127.0.0.1:8000/download/v1.1.0-test2/marstek-monitor-1.1.0-test2-macos.dmg"},
       {"name": "marstek-monitor-1.1.0-test2-macos.dmg.sha256",
        "browser_download_url": "http://127.0.0.1:8000/download/v1.1.0-test2/marstek-monitor-1.1.0-test2-macos.dmg.sha256"}
     ]
   }
   ```

3. Запустити сервер (лише на loopback):

   ```sh
   cd "$SRV" && python3 -m http.server 8000 --bind 127.0.0.1
   ```

4. В іншому терміналі запустити test1 через `open` (не з терміналу: тоді
   дозвіл Local Network і сповіщення належать самій програмі):

   ```sh
   open -n --env MARSTEK_UPDATE_BASE=http://127.0.0.1:8000/ \
     --env MARSTEK_UPDATE_API=http://127.0.0.1:8000/latest.json \
     ~/Applications/MarstekTest/"Marstek Monitor.app" \
     --args -simulate-quiet -simulate-dir "$(mktemp -d)"
   ```

   У журналі тимчасової теки має бути `update: local update test`.

5. Налаштування → Про програму → «Перевірити зараз» → «Оновити». Очікується:
   - програма закривається, з того самого шляху відкривається test2;
   - «Про програму» показує 1.1.0-test2;
   - у тимчасовій теці даних є `backups/monitor-1.1.0-test1.db`, дані
     симуляції на місці;
   - після старту test2 поруч із програмою немає `Marstek Monitor.app.old` і
     тек `.marstek-update-*`.

6. Негативні перевірки (кожна — з нового test1):
   - test2, підписаний ad-hoc або іншою командою: помилка перевірки підпису,
     test1 не змінено, `.old` не з'явився;
   - програма, запущена прямо з підключеного dmg (`/Volumes/…`) або з
     «Завантажень» (Gatekeeper переносить її в AppTranslocation): лише
     «Завантажити», без «Оновити»;
   - контрольна сума в `.sha256` змінена: помилка контрольної суми, нічого не
     встановлено.

7. Прибрати: закрити програму, зупинити сервер, зняти обидві тестові збірки з
   реєстру LaunchServices і видалити теку:

   ```sh
   LSR=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
   $LSR -u ~/Applications/MarstekTest/"Marstek Monitor.app"
   $LSR -u "$PWD/bin/marstek-monitor.app"
   rm -rf ~/Applications/MarstekTest "$SRV"
   ```

   Інакше клік по сповіщенню Marstek згодом може відкрити тестову копію.
