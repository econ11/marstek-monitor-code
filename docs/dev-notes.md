# Dev notes

## Toolchain (verified 2026-11-21)

- Wails: **v3.1.0** (stable; `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`, `wails3 doctor` clean)
- Go: 1.26.3 (go.mod: `go 1.26.0`), Node: 24.13.1, npm 11.8.0
- Module: `github.com/econ11/marstek-monitor-code`
- Build: `wails3 build` -> `bin/marstek-monitor.exe` (production, `-H windowsgui`). Dev: `wails3 dev`.
- Template name in v3.1.0 is `vanilla` (TypeScript); `vanilla-ts` is not a template (the CLI treats unknown names as a remote git URL). `wails3 init -d .` still creates a subdirectory; its contents were moved to the repo root.
- Bindings are generated on build into `frontend/bindings` (gitignored); frontend imports e.g. `../bindings/github.com/econ11/marstek-monitor-code/app`.

## Wails v3.1.0 API names used

- App: `application.New(application.Options{Name, Description, Services, Assets})`, `a.Run()`, `a.Quit()`
- Services: `application.NewService(&app.Service{})` (exported methods bound to the frontend, e.g. `Service.Ping`)
- Window: `a.Window.NewWithOptions(application.WebviewWindowOptions{Title, Width, Height, URL, BackgroundColour})`; `win.Show() / Hide() / UnMinimise() / Focus()`
- Hide on close: `win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent){ win.Hide(); e.Cancel() })` (pkg `.../pkg/events`)
- Tray: `a.SystemTray.New()` -> `*SystemTray`; `.SetIcon([]byte png)`, `.SetTooltip`, `.SetMenu(menu)`, `.OnClick(func())`, `.OnRightClick`
- Menu: `a.NewMenu()`, `menu.Add(label).OnClick(func(*application.Context))`, `menu.AddSeparator()`
- Notifications: `notifications.New()` (pkg `.../pkg/services/notifications`) registered as a service; `svc.SendNotification(notifications.NotificationOptions{ID, Title, Body})` (also `SendNotificationWithActions`, `RemoveNotification(id)`, `OnNotificationResponse`)
- Events (Go -> JS): `a.Event.Emit(name, data)`; Go listeners: `a.Event.On(name, func(*CustomEvent))`; typed events registered with `application.RegisterEvent[T](name)` in `init()`; JS side `Events.On(name, cb)` from `@wailsio/runtime`
- App lifecycle event: `a.Event.OnApplicationEvent(events.Common.ApplicationStarted, ...)`

## Manual check on Windows 11 (Task 1)

- Toast "Marstek Monitor / тест" appeared bottom-right at startup: works.
- Window renders frontend and Go binding (`Service.Ping` -> "pong"): works.
- Closing the window (WM_CLOSE) hides it, process stays alive: works.
- Tray icon: registered (HKCU\Control Panel\NotifyIconSettings entry for the exe). On this machine it lives in the taskbar overflow; flyout could not be opened reliably by synthetic input, so left-click-show and the "Вийти" menu item were not exercised by hand (code paths are straightforward).
- No fallback to Wails v2 was needed.
- Tray icon: the app icon (on/off with the grid, same rule as the window icon); the details are in the tooltip and the menu. `app/trayicon` builds the tooltip text and the short formats (`ShortDuration`, `Decimal`, `GroupInt`) that `app/toast.go` and `app/journaltext.go` also use.
- Toast on Windows registers an AppUserModelID under HKCU\Software\Classes\AppUserModelId using the app name.

## Build assets pruned (Task 1 fix rounds)

Removed: `build/android`, `build/ios`, `build/darwin`, `build/linux`, `build/docker`, and `build/windows/msix` (MSIX is not needed for v1; NSIS is the Windows installer). Also removed the tasks that depended on them: docker/ios tasks in `build/Taskfile.yml`, the macOS icon output of `generate:icons`, the MSIX package task and `install:msix:tools`, and the platform includes in the root `Taskfile.yml`. The Docker cross-compile branch in `build/windows/Taskfile.yml` is still present but only used for non-Windows hosts with CGO (not our case); its hint now says the Docker assets were removed.
Regenerate with `wails3 update build-assets` when a macOS project starts (then restore includes). Product metadata lives in `build/config.yml`; after editing it run `wails3 task common:update:build-assets`, which re-creates the removed dirs, so prune again.

## App icon

The app icon (H2, two states) is drawn by hand: sources and README in `docs/design/logo/`. `build/windows/icon.ico` is `appicon-on.ico` (16-64 px BMP hinted from the small masters, 256 PNG) and goes into the exe through `generate:syso` on every `wails3 build`; the NSIS installer uses the same file. `build/appicon.png` is the 1024 px "on" render, kept as the master for other platforms. The `generate:icons` task was removed: it rebuilt `icon.ico` from `appicon.png` and would have thrown away the hinted small sizes. `app/assets/` holds the run-time copies (window icon on/off, toast logos).

## Fresh clone

`main.go` embeds `frontend/dist`, which is gitignored: run `npm install` in `frontend/` and `wails3 build` (or `npm run build`) before plain `go build`/`go vet`. Minimum Go toolchain: 1.26.0 (go.mod).

## Як запустити зонд

Зонд `marstek-probe` записує сирі відповіді станції (Venus E 3.0) у файл `.jsonl`. Він ніколи не викликає `Bat.GetStatus`.

1. Зберіть: `go build -o bin/marstek-probe.exe ./cmd/marstek-probe`
2. Запустіть у PowerShell у теці, куди писати лог: `.\bin\marstek-probe.exe` (станцію буде знайдено автоматично; якщо їх кілька або виявлення не спрацювало — `-ip 192.168.x.y`). Інші прапорці: `-interval 5s`, `-out файл.jsonl` (за замовчуванням `probe-ГГГГММДД-ГГММСС.jsonl`).
3. Залиште працювати щонайменше 24 години (ПК не повинен засинати: вимкніть сон у налаштуваннях живлення). Раз на 5 хв у консолі з'являється рядок-підсумок (відповіді, таймаути, `bat_soc`, `ongrid_power`, `offgrid_power`). Ctrl+C — зупинка.
4. За цей час станція має хоча б раз лишитися без мережі: справжнє відключення світла або вимкнення автомата на вході станції на ~5 хв — і з навантаженням, і без нього. Запишіть собі приблизний час відключень.
5. Надішліть файл `.jsonl` (і часи відключень).

## Venus E 3.0 v151: знахідки

Джерело: лог зонда `probe-20261121-130444.jsonl` (21.11.2026), нотатки власника.

- Хронологія: 13:09:00 мережу вимкнено (будинок живиться від станції через АВР); чайник 13:12:00–13:14:54 та 13:17:00–13:21:47 (`offgrid_power` ≈ 2240–2250 Вт); базове навантаження ≈ 310–430 Вт; 13:25:10 мережа повернулась, 13:25:15 `offgrid=0`, `ongrid` = −2341…−2492 (заряд на максимумі); 13:34:36 повна, `ongrid=0`, `offgrid=0`.
- Знак: `ongrid_power` від'ємний = імпорт з мережі. `bat_power` на прошивці 151 відсутній; `pv_power` = 0 (PV немає).
- Фікстури: `es_status_grid_off_kettle.json` (13:12:25), `es_status_grid_on_charging.json` (13:26:06), `es_status_grid_on_idle.json`.

### Правило мережі (затверджено власником 2026-11-21)

- Мертва зона `GridDeadbandW = 20` Вт.
- `GridAbsent` тоді й лише тоді, коли `offgrid_power > 20` **і** `ongrid_power >= -20` (обидва не nil).
- `GridPresent`, якщо `ongrid_power < -20` (не nil) **або** `offgrid_power <= 20` (не nil).
- Інакше `GridUnknown` (напр. обидва nil, або відомий лише `ongrid` у [-20, +∞) при `offgrid` nil).
- Пріоритет: спершу Present за зарядом (`ongrid < -20`), потім Absent, потім Present за простоєм (`offgrid <= 20`), інакше Unknown.

Потужність батареї: `BatteryW = −(ongrid + offgrid − pv)`, `pv` nil = 0; якщо `ongrid` або `offgrid` nil — запасний варіант `−bat_power` (знак НЕ перевірено на реальних даних), інакше nil.

### Застереження АВР

Якщо АВР вручну залишити на «батарея», коли мережа є і SOC=100, правило покаже `GridAbsent` (спостерігалось 13:08:34–13:09). Прийнято.

### Обмеження API

Запити поспіль дають таймаути; пауза ≥ 1 с між викликами це лікує. Під великим навантаженням іноді трапляється відповідь `-32700 Parse error` (при цьому `ES.GetStatus` усе одно приходить кожні ~5–10 с).

### `Bat.GetStatus`: додаток його не викликає (рішення власника 2026-11-23, Ruling 57)

На деяких прошивках (за спільнотою — 138–148) `Bat.GetStatus` перезавантажує станцію. Контрольований тест 2026-11-22 на Venus E 3.0 v151 (три виклики) минув без перезавантаження, тож план 2 (Task 15b, 17, Rulings 53/56) читав його раз на 10 хв. Але потім на тій самій v151 станція тричі замовкала понад 60 с одразу після виклику (2026-11-22 о 19:19, 21:12 і 21:57) і перезапустилась ≈ 00:25 2026-11-23 невдовзі після старту додатка з «Дані батареї: Авто»; перезапуск станції може знеструмити будинок (АВР). Тому власник вирішив прибрати запит зовсім: жоден шлях коду його не надсилає (сторож — `core/monitor` `TestMonitorNeverSendsBatGetStatus`), зонд `marstek-probe` теж. Температури, енергії в батареї, ємності «за даними станції» і прапорців BMS у додатку немає; ємність для прогнозів — номінал моделі, здоров'я — з розрядів (Task 14). Що лишилось від старих версій: таблиця `bat_status` (міграція v4, схему не змінюємо) — не пишеться й не читається, Prune видаляє її рядки старші за термін зберігання; поле `BatteryDetails` у `app.settings` ігнорується й зникає з наступним збереженням; внутрішні ключі `battery.last_call`, `battery.verified`, `battery.verify` більше не читаються й не пишуться (рядки не видаляються); старі записи журналу `battery.readsStopped/readsResumed/fwUnverified/fwVerified` і `station.chargeBlocked/…Allowed/dischargeBlocked/…Allowed` показуються своїм текстом без поради.

Global Constraints плану 2 (правило «`Bat.GetStatus` — лише за правилами Task 15b») — історичні: діє заборона вище, `Bat.GetStatus` не викликається ніколи.

## Шар застосунку (Task 12)

### Як перевірити вручну без станції

`bin\marstek-monitor.exe -simulate` піднімає у процесі несправжню Venus E на 127.0.0.1 (справжню станцію не чіпає), тимчасову БД і власний mutex, тож може працювати поруч зі справжнім додатком. Автозапуск у цьому режимі не реєструється. Сценарій один раз від старту: 0:00 світло є, 95%; 1:00 світло зникає (навантаження 400 Вт); 3:00 світло з'являється, заряд 2492 Вт; ≈6:00 100%. Через дебаунс 30 с тости приходять приблизно на 1:35 («Зникло світло»), 3:35 («Світло з'явилось») і 6:05 («Станція заряджена»). Журнал з кожною зміною іконки, тултіпа й меню: `%TEMP%\MarstekMonitor-simulate-*\monitor.log`. Додайте `--background`, щоб перевірити старт без вікна.

У режимі імітації кожен тост має заголовок з префіксом «[Імітація] », і так само починається тултіп іконки в треї — щоб несправжнє «Зникло світло» не сплутати зі справжнім. **Автоматичні перевірки (агенти, скрипти) запускайте з `-simulate-quiet`** (поєднується з `-simulate-dir`): тости тоді йдуть одразу в Центр сповіщень, без банера і звуку.

Повний сценарій `-simulate` (час від старту процесу):

| Час | Що робить несправжня станція | Що видно в додатку |
|---|---|---|
| 0:00 | світло є, 95%, 0 Вт | очікування; перші ~30 с «Світло: невідомо» і сіра пунктирна рейка |
| 1:00 | світло зникає, навантаження 400 Вт | третє кільце «До розряду» («рахуємо…» близько хвилини); з ≈1:35 жовта рейка, заголовок вікна «Без світла …», тост «Зникло світло» |
| 3:00 | світло з'являється, заряд 2492 Вт | третє кільце «До повного»; з ≈3:35 рейка гасне, тост «Світло з'явилось» |
| ≈6:00 | 100%, 0 Вт | очікування, тост «Станція заряджена» |
| 6:30 | перестає відповідати на 6 хв (і на виявлення теж) | з ≈6:45 STALE: банер «Дані застарілі…», кільця сірі, прогноз «—»; з ≈11:45 NO_LINK: банер «Немає зв'язку зі станцією», пунктирна рамка на графіку |
| 12:30 | знову відповідає | живі дані, на графіку лишається пропуск «немає зв'язку» |

Картка «Поки додаток не працював»: запустіть `bin\marstek-monitor.exe -simulate-dir C:\Temp\mm-sim` (БД і журнал лишаються в цій теці між запусками), дайте попрацювати хвилину, вийдіть через меню трею («Вийти»), зачекайте понад 5 хв і запустіть ту саму команду ще раз. Картка з'являється над кільцями, щойно надійдуть перші дані; після «×» для того самого пропуску вона не повертається. Щоб почати з чистого аркуша, видаліть теку. LIKELY_DEAD у симуляторі не відтворюється (потрібен розряд до нижньої межі без світла).

Звичайний запуск пише журнал у `%LOCALAPPDATA%\MarstekMonitor\monitor.log` (починається заново після 1 МБ).

### Wails v3.1.0: що використано додатково

- Вікно без показу на старті: `WebviewWindowOptions.Hidden`; не завершувати додаток, коли вікно сховане: `Options.Windows.DisableQuitOnLastWindowClosed`.
- Трей: `tray.OnDoubleClick`, `menu.AddSubmenu(label)`, `item.SetEnabled(false)`; оновлення меню — `menu.Clear()`, додати пункти; `tray.SetMenu(menu)` лише раз до `app.Run`, далі `menu.Update()` (інакше гонка старту Wails; перебудова відкладається, поки меню відкрите). Перереєстрацію іконки після перезапуску Explorer (`TaskbarCreated`) Wails робить сам.
- Іконка трею — іконка застосунку on/off із вбудованих ICO, розмір за DPI системи; тема панелі задач більше не впливає (`SystemThemeChanged` і `SystemUsesLightTheme` не використовуються).
- Сон і пробудження: див. нижче.
- Клік по тосту: `notifications.Service.OnNotificationResponse`; `Response.UserInfo` несе `station_id`, `intent`, `event_id`.
- Вибір теки (CSV-експорт, Task 16): `application.Get().Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).PromptForSingleSelection()` — на Windows це діалог «Вибрати теку» (`openSingleFolder`); скасування повертає `""` без помилки. «Показати в папці» — не Wails: `explorer.exe /select,"<шлях>"` через `SysProcAttr.CmdLine` (лапки всередині аргументу, яких `exec` сам не ставить).
- Одна копія: власний mutex `Local\MarstekMonitor` і подія `Local\MarstekMonitor.show` (`app/window.go`), не `Options.SingleInstance` — щоб ім'я mutex було саме таким.

### Тости: власний виклик WinRT

Тости надсилає `app/toastpush.go` напряму через Windows Runtime (go-ole), а не сервіс Wails і не wintoast. Причини:

- **Емодзі.** `go-ole.NewHString` передає довжину рядка в рунах, а не в одиницях UTF-16, тому кожен символ поза BMP (усі наші емодзі) відрізає один символ з кінця XML, і Windows відхиляє документ (`doc.LoadXml … 0xC00CE55F`). Ми створюємо HSTRING самі (`WindowsCreateString` зі справжньою довжиною).
- **Тег і група** (`IToastNotification2.Tag/Group`): «Зникло світло» і «Світло з'явилось» одного відключення мають `Tag = outage-<id>`, `Group = id станції`, тож другий тост замінює перший у Центрі сповіщень. Інші види: `Tag = вид` (один запис на вид і станцію).
- **Без спливання** (`IToastNotification2.SuppressPopup`): тиха година і «Не турбувати» кладуть тост одразу в Центр сповіщень, без банера і звуку. «Низький заряд» не приглушується ніколи.

Сервіс Wails лишається для реєстрації додатка (AppUserModelId, COM-активатор) і для кліку (`OnNotificationResponse`); `launch` тоста ми кодуємо у форматі, якого він чекає (`notifications.NotificationPayload`, base64). Усі виклики WinRT ідуть з одного потоку ОС (`onWinRT`), ініціалізованого `RoInitialize`. Номери слотів vtable та IID перевіряє тест `TestNewToastSetsTagGroupAndSuppressPopup`: він створює справжній `ToastNotification` (нічого не показуючи) і читає властивості назад.

Історія сповіщень (`ToastNotificationManager.History`) властивість `SuppressPopup` не зберігає — у прочитаному назад записі вона завжди `False`; ознака приглушеного тоста там — `<audio silent="true"/>`.

`-simulate-quiet` — те саме, що `-simulate`, але з тихою годиною на всю добу: тости мають з'являтися лише в Центрі сповіщень.

`-simulate-firstrun` — `-simulate`, що не додає несправжню станцію сам (`Deps.AutoAdd` вимкнено): з порожньою БД монітор не стартує й не звертається до мережі, а станцію знаходить екран першого запуску (Task 18b; несправжня станція — єдина ціль пошуку). Поєднується з `-simulate-quiet` і `-simulate-dir` (якщо в теці вже є станція, прапор нічого не змінює).

`-simulate-e2e` — `-simulate-firstrun`, у якому екран першого запуску сам проходить сценарій (Task 19, `frontend/src/views/firstRunE2E.ts`): пошук → IP 127.0.0.2 (немає відповіді, інша підмережа) → «1.2.3» (формат) → пошук знову → «Додати» → DONE → Огляд. Крок видно в заголовку вікна («Marstek Monitor [e2e] N …»), тож агент читає його (GetWindowText) і знімає вікно (PrintWindow) без жодного вводу. У журналі — `station added` і `first run handed over sinceLastCall=…` (≥ 1,1 с). Лише для перевірок.

### Сон і пробудження

`WM_POWERBROADCAST` перехоплює `Options.Windows.WndProcInterceptor` (`app/power.go`) і ставить події в чергу, яку розбирає одна горутина. Події Wails (`events.Windows.APMSuspend/APMResumeAutomatic`) не використовуємо: Wails запускає обробник кожної події в новій горутині, і після пробудження «сон» і «пробудження» могли б виконатись у зворотному порядку.

### Виявлення станції

Запит іде на 255.255.255.255 і на directed broadcast кожного активного IPv4-інтерфейсу (`lan.BroadcastTargets`). Станція губить приблизно кожен третій запит виявлення (заміряно 21.11.2026: 9 з 12 відповідей на всі адреси, 8 з 12 лише на directed broadcast домашньої підмережі), тому перший пошук повторюється через 5, 10, 20, 40 с, далі щохвилини.

### Перевірка «рухомого стека» для викликів WinRT

Вихідні параметри системних викликів у `app/toastpush.go` передаються як `uintptr(unsafe.Pointer(&v))`. Функції `call` і `callWithString` позначені `//go:uintptrescapes`: без цього `v` лишається на стеку горутини, Go може перемістити стек перед викликом, і Windows запише результат у стару пам'ять (далі — nil і паніка). Звичайні тести цього не ловлять. Запускати після будь-якої зміни в `toastpush.go` чи іншому коді з `SyscallN`:

```
wails3 task test:stackmove
```

або напряму: `go test -count=1 "-gcflags=github.com/econ11/marstek-monitor-code/app=-d=maymorestack=runtime.mayMoreStackMove" ./app/`. Прапорець змушує рантайм переміщати стек перед кожним викликом функції в пакеті `app`.

Робоча горутина WinRT (`onWinRT`) ловить паніку, пише її в журнал і живе далі: збій тоста не має завершувати додаток.

## Адаптивне опитування (план 2, Task 2b)

Станція губить запити поспіль і нестабільна під навантаженням, а найчастіший стан — «повна, світло є» — нічого не змінює годинами. Тому `core/monitor` має два режими (`core/monitor/cadence.go`), значення погоджено власником:

- **Active** — інтервал з `Deps.PollInterval` (з Task 4b — налаштування «Опитування локально»; за замовчуванням 5 с).
- **Calm** — `max(20 с, Active)`. Лише коли на поточному зразку одночасно: зв'язок LIVE, світло підтверджено (і зразок показує його), батарея в очікуванні (|P| ≤ 20 Вт), немає непідтвердженої зміни мережі (`detect.Tracker.Pending`) чи режиму батареї, SOC за останню хвилину в межах 1 п.п., і минуло ≥ 2 хв від старту або пробудження.
- З Calm у Active — одразу на зразку, що показав відсутність світла, навантаження чи зміну SOC: наступний запит через активний інтервал від цього зразка. Найгірша затримка виявлення відключення в Calm — ≤ 20 с + 30 с дебаунсу.
- «Свіже» (`link.Machine.SetFresh`) = 3 × поточний інтервал: 15 с в Active, 60 с у Calm. STALE → NO_LINK лишається 5 хв **після** «свіжого», тож у Calm NO_LINK настає приблизно через 6 хв замість ≈ 5 хв 15 с. Це прийнято.
- Бекоф при збоях рахується від активного інтервалу (5→10→20→40→60 с), режим при збої не змінюється (інакше один загублений запит у Calm одразу давав би STALE). Між будь-якими двома викликами — ≥ 1 с, як і раніше.
- Подія `monitor.cadence` (info, текст `calm`/`active`) пишеться в журнал БД лише при зміні режиму; це налагоджувальна подія, не для журналу «Помилки».
- Наслідок для пізніших задач: крок «без дірки» між зразками — 60 с (3 × 20 с), не 15 с.

У сценарії `-simulate` режим Calm може настати після ≈6:30 (100%, 0 Вт, SOC стабільний хвилину); якщо до мовчання станції він уже настав, STALE з'явиться на ≈7:30 замість ≈6:45, а NO_LINK — на ≈11:30.

## CSV-експорт (план 2, Task 16)

- Діалект — за мовою застосунку (`export.FormatFor(i18n.Lang())`): uk — роздільник `;` і десяткова кома (Excel з українськими регіональними налаштуваннями ділить саме за `;`), en — `,` і крапка. BOM UTF-8 є завжди; рядок `sep=` не пишемо (з ним Excel ігнорує BOM). Час — RFC 3339 з місцевим зсувом, заголовки англійською в обох мовах.
- Межа терміну зберігання — `monitor.RetentionBound`: місцева північ N місцевих діб тому, тож найстаріша доба в «Відключеннях» і «Енергії» ціла.

## Приймання плану 2

Власник перевірив усе на справжній станції Venus E 3.0 v151, 2026-11-22…23, збірки до `bf3a945`.

### Автоматичні перевірки

`go vet`, `go test -race ./...`, `vitest`, typecheck, `wails3 build` — проходять.

### Ручні перевірки (пройдено)

- Ранок після ночі з вимкненим ПК: «≥», «є пропуски в даних», без хибного «Світло з'явилось».
- «Відключення» за 7 днів збігаються з відомими; ніч без ПК заштрихована.
- «Помилки» (події); «Енергія» — величини як в офіційному додатку; «Батарея» — «≈ N» циклів / «ще рахуємо».
- Адаптивне опитування (див. «Адаптивне опитування»): ~20 с у спокої, ~5 с інакше, без «застарілих» даних — видно за «N с тому» в панелі додатка; вік після сну й пробудження правильний.
- Трей: меню (виправлена гонка старту Wails), іконка = іконка застосунку, on/off; підказки ⓘ; uk↔en наживо; клавіатура.
- Налаштування (оновлення на місці; підтвердження читання батареї прибране разом із функцією), CSV-експорт (див. «CSV-експорт»).
- Перший запуск: автовиявлення і ручний IP.
- Вимкнення і вмикання світла автоматом: стан «перевіряємо», тости, іконки.

### Зміни під час приймання

- `Bat.GetStatus` прибрано повністю: станція тричі замовкала й перезапустилась — рішення власника, не повертати (див. «`Bat.GetStatus`: додаток його не викликає»).
- Іконка трею = іконка застосунку.
- Очікуваний стан мережі «перевіряємо…».
- «Підтримати автора».
- Підпис заряду «5,12 з 5,12».
- Графік показує мінімум станції замість порогу сповіщення; «Поріг сповіщення» — у налаштуваннях.
- Відключення, що почалось у пропуску, показується як «між {from} і {to}» (тост, «Огляд», «Відключення»).
- Старі записи журналу батареї показуються як інформаційні.

### Перенесено в план 3

- Журнал: короткі обриви зв'язку (час + тривалість).
- Вкладка «Помилки»: посторінковість, без повного перезавантаження раз на 60 с.
- Коди помилок брандмауера замість regex.
- Тест паритету текстів Go↔TS.
- Англійський CSV: «Sep»/«Sept».
- Глибокі ланцюги перекритих пропусків можуть обрізатися в знімку.
- Кілька станцій в UI, інсталятор, перевірка оновлень.
- Повний порт на macOS: триває (spike і піктограми рядка меню готові), оцінка 7–10 днів.

## Версія збірки

`wails3 build` без VERSION дає `Version = "dev"` (у «Про програму» — «збірка розробника»). `wails3 build VERSION=1.2.3` (так само `task build VERSION=1.2.3`, якщо встановлено `task`) вшиває версію: `build/windows/Taskfile.yml` передає `-X '<модуль>/app.Version=…'`, де `<модуль>` береться з `go list -m`, тож перейменування модуля нічого не ламає. VERSION може містити лише літери, цифри та `. _ + -`, інакше збірка зупиняється. `SettingsView.Platform` — це `runtime.GOOS`.

Збірка розробника (`dev` або порожня версія) не перевіряє оновлення взагалі: ні раз на день, ні кнопкою, і «Оновити» не працює; у «Про програму» рядок «Збірка розробника — оновлення не перевіряються.» (фінальне рев'ю плану 3, M2).

## Вкладка «Помилки»: читання періоду (рішення для v1)

`JournalPage` / `JournalSince` (`core/monitor/reports.go`) на кожен виклик (сторінка, оновлення раз на 60 с) читають і будують весь вибраний період у ядрі, а потім рахують, фільтрують і ріжуть у пам'яті; для v1 це прийнято (фінальне рев'ю плану 3, I2, рішення 6), бо журнал містить лише події з кодами, пропуски й короткі обриви — кілька записів на день, а RPC, JSON і DOM вже посторінкові; пейджинг у сховищі (`WHERE (at,id) < ? … LIMIT ?` + `COUNT … GROUP BY`) — після v1, якщо журнал виросте.
