# macOS: підпис, нотаризація і тест оновлення

Збирає Mac-реліз `scripts/macos-package.sh <версія> [<ідентичність>]`: universal
`.app` (arm64 + x86_64) зі штампом версії, `bin/marstek-monitor-<версія>-macos.dmg`
і `.sha256`. З ідентичністю `"Developer ID Application: <name> (<TEAMID>)"`
програма й dmg підписуються з hardened runtime і міткою часу; коли задано
`NOTARY_KEY_ID`, `NOTARY_ISSUER` і `NOTARY_KEY_PATH`, обидва нотаризуються й
степляться. Без ідентичності все підписано ad-hoc — така програма не може
оновити сама себе (немає Team ID), їй пропонується лише «Завантажити».

## Налаштування підпису (один раз)

Робиться один раз на Mac власника, коли членство в Apple Developer Program уже
активне. Результат — шість секретів у `econ11/marstek-monitor-code` (Settings →
Secrets and variables → Actions), з якими GitHub Actions підписує, нотаризує й
публікує реліз (`docs/releasing.md`). Жодне значення не вставляється в чат, у
issue чи в коміт: секрети йдуть з файлів або з прихованого запиту `gh` просто
в GitHub. Назви пунктів меню наведено англійською, як в інтерфейсі Apple і
GitHub; в українській macOS вони перекладені, а деякі назви можуть трохи
відрізнятись від версії до версії.

### 1. Сертифікат Developer ID Application

Найпростіше — через Xcode (безкоштовно в App Store):

1. Xcode → **Settings…** → вкладка **Accounts** → **+** внизу → **Apple ID** →
   увійти тим Apple ID, на який оформлено членство.
2. Вибрати свою команду (Team, позначка «Individual» або ваше ім'я) →
   **Manage Certificates…**.
3. **+** внизу ліворуч → **Developer ID Application**. Xcode сам створить ключ
   і сертифікат і покладе обидва в зв'язку ключів `login`.
4. Перевірити в Терміналі:

   ```sh
   security find-identity -v -p codesigning
   ```

   У списку має бути рядок `"Developer ID Application: … (…)"`. Його не треба
   нікуди копіювати.

Якщо пункту **Developer ID Application** немає: створювати такі сертифікати
може лише Account Holder (для індивідуального членства це ви), і членство має
бути вже активне. Шлях без Xcode (довший):

1. **Keychain Access** (Cmd+Пробіл, «Keychain Access» або «Доступ до в'язки
   ключів»; у нових macOS його немає в «Утилітах», але Spotlight знаходить) →
   меню **Keychain Access → Certificate Assistant → Request a Certificate From
   a Certificate Authority…** → ваш e-mail і ім'я, **Saved to disk** →
   зберегти файл `.certSigningRequest`.
2. <https://developer.apple.com/account> → **Certificates, IDs & Profiles** →
   **Certificates** → **+** → **Developer ID Application** → (якщо питає тип
   — **G2 Sub-CA**) → завантажити файл запиту → **Download** → двічі клацнути
   завантажений `.cer`, щоб він ліг у зв'язку `login`.
3. Перевірити так само, `security find-identity -v -p codesigning`.

Цей самий сертифікат потрібен і для розділу «Тест оновлення перед релізом»
нижче.

### 2. Експорт сертифіката в `.p12`

1. Відкрити **Keychain Access** (як вище).
2. Ліворуч зв'язка **login**, угорі вкладка (або категорія) **My Certificates**
   (назва пункту може трохи відрізнятись).
3. Знайти **Developer ID Application: …**. Біля нього має бути трикутник, під
   яким лежить приватний ключ: без ключа експорт марний.
4. Виділити рядок **сертифіката** (не лише ключа) → правою кнопкою → **Export
   "Developer ID Application: …"…**.
5. Формат **Personal Information Exchange (.p12)**, назва, наприклад,
   `developer-id.p12`, місце — Робочий стіл.
6. Задати пароль експорту: довгий, згенерований менеджером паролів, **не
   порожній**. Це значення секрету `MACOS_CERT_PASSWORD`.
7. macOS попросить пароль входу в систему, щоб дозволити експорт ключа →
   **Allow**.

### 3. Ключ App Store Connect API для нотаризації

1. <https://appstoreconnect.apple.com> → **Users and Access** → вкладка
   **Integrations** → ліворуч **App Store Connect API** → **Team Keys** (назва
   пункту може трохи відрізнятись).
2. Якщо це перший ключ, спочатку **Request Access** і погодитись з умовами
   (робить Account Holder).
3. **Generate API Key** (або **+**) → назва, наприклад, `notary` → Access
   **Developer** → **Generate**.
4. У таблиці з'явиться ключ: його **Key ID** — це `NOTARY_KEY_ID`. Над
   таблицею показано **Issuer ID** — це `NOTARY_ISSUER`. Обидва можна
   відкрити тут знову будь-коли.
5. **Download API Key** → файл `AuthKey_<Key ID>.p8` у «Завантаженнях».
   **Завантажити його можна лише один раз.** Якщо файл загубився — відкликати
   ключ (**Revoke**) і створити новий.

### 4. Токен GitHub для репозиторію релізів

1. github.com → аватар → **Settings** → **Developer settings** → **Personal
   access tokens** → **Fine-grained tokens** → **Generate new token**.
2. Назва, наприклад, `marstek-monitor releases`. **Expiration**: Custom, рік
   від сьогодні. Одразу поставити собі нагадування в календарі за два тижні до
   цієї дати: з простроченим токеном реліз збирається, але job **publish**
   падає. Поновлення = новий токен і знову `gh secret set RELEASES_TOKEN`.
3. **Resource owner**: ваш акаунт. **Repository access**: **Only select
   repositories** → лише `econ11/marstek-monitor` (репозиторій релізів, не
   коду).
4. **Permissions → Repository permissions → Contents: Read and write**.
   **Metadata: Read-only** додається сам. Більше нічого не вмикати.
5. **Generate token**. Токен показується один раз: не зберігати його у файл,
   а одразу вставити в запит `gh` з кроку 5 (`RELEASES_TOKEN`).

### 5. Шість секретів через `gh`

Потрібен `gh` (`brew install gh`, потім `gh auth login`). Команди не містять
значень, тож і в історії терміналу їх не буде. Файли — на Робочому столі та в
«Завантаженнях», як вище; `AuthKey_XXXX.p8` замінити справжньою назвою файлу.

```sh
R=econ11/marstek-monitor-code

# Сертифікат з ключем, base64 одним рядком (macOS base64 не переносить рядки)
base64 -i ~/Desktop/developer-id.p12 | gh secret set MACOS_CERT_P12 --repo $R

# Файл ключа API як є
gh secret set NOTARY_KEY_P8 --repo $R < ~/Downloads/AuthKey_XXXX.p8

# Ці чотири gh запитає сам («Paste your secret»), введене не показується:
gh secret set MACOS_CERT_PASSWORD --repo $R   # пароль експорту .p12
gh secret set NOTARY_KEY_ID --repo $R         # Key ID з App Store Connect
gh secret set NOTARY_ISSUER --repo $R         # Issuer ID з App Store Connect
gh secret set RELEASES_TOKEN --repo $R        # токен з кроку 4

gh secret list --repo $R
```

`gh secret list` має показати рівно ці шість назв: `MACOS_CERT_P12`,
`MACOS_CERT_PASSWORD`, `NOTARY_KEY_ID`, `NOTARY_ISSUER`, `NOTARY_KEY_P8`,
`RELEASES_TOKEN`. Значення GitHub не показує нікому, і це нормально.
Помилилися — просто виконати ту саму команду ще раз, вона перезапише секрет.

### 6. Прибрати файли

```sh
rm ~/Desktop/developer-id.p12 ~/Downloads/AuthKey_XXXX.p8
```

Також можна видалити `.certSigningRequest` і `.cer`, якщо йшли шляхом без
Xcode, і очистити Смітник. За бажання спершу покласти `.p12` (з його паролем)
і `.p8` у менеджер паролів як резервну копію. Сам сертифікат лишається в
зв'язці ключів, тож `.p12` можна експортувати знову будь-коли; `.p8` — ні.

### 7. Пробний прогін (dry run)

Розділ `## 1.1.0` у `CHANGELOG.md` має вже бути на `main`, інакше job
**prepare** впаде.

1. github.com → `econ11/marstek-monitor-code` → **Actions** → ліворуч
   **release** → **Run workflow**.
2. Branch `main`, version `1.1.0`, **dry_run** — увімкнено (так за
   замовчуванням) → **Run workflow**.

   Або з терміналу:
   `gh workflow run release.yml --repo econ11/marstek-monitor-code -f version=1.1.0 -f dry_run=true`.
3. Очікується: **prepare**, **windows**, **macos**, **intel-check** зелені,
   **publish** — сірий (skipped). Job **macos** чекає на нотаризацію Apple,
   це кілька хвилин.

Якщо червоне:
- **Signing secrets present** — бракує секрету; назва є в повідомленні.
- **the certificate has no Developer ID Application identity** — у `.p12`
  немає ключа або експортовано не той сертифікат (крок 2).
- помилка `security import` — пароль `.p12` не той (`MACOS_CERT_PASSWORD`).
- помилка `notarytool` — не збігаються Key ID, Issuer ID і файл `.p8`, або
  ключ відкликано (крок 3).

Нічого з цього не публікує реліз: dry run лише збирає й перевіряє.

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
