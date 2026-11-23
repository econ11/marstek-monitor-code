# Changelog

## 1.1.0

### Українською

Перший випуск Marstek Monitor для macOS. Windows-версія працює як і раніше.

- macOS 12 і новіші, одна програма для Apple Silicon та Intel. Встановлення: відкрити `marstek-monitor-1.1.0-macos.dmg` і перетягнути додаток у «Програми».
- Програма підписана й нотаризована Apple. Під час першого запуску macOS, як для будь-якої програми з інтернету, один раз попросить підтвердити відкриття.
- Ті самі вкладки й сповіщення, що й на Windows. Іконка живе в рядку меню й показує, чи є світло; іконка в Dock з'являється, лише поки відкрите вікно.
- Клік по сповіщенню відкриває вікно на потрібній вкладці.
- «Відкривати під час входу в систему» в налаштуваннях. Дані зберігаються в `~/Library/Application Support/MarstekMonitor`.
- Додаток просить дозвіл на доступ до локальної мережі; якщо його немає, підказує, де ввімкнути.
- Оновлення в один клік і на Mac: завантаження, перевірка SHA-256 і підпису розробника, резервна копія бази, заміна програми й перезапуск. Працює, коли додаток лежить у «Програмах»; інакше пропонується «Завантажити».
- Релізи тепер збираються на GitHub, і в кожному є файли для обох систем.
- Windows: помітних змін немає. Перевірка оновлень бере з релізу лише інсталятор для Windows, а не файл для Mac.

### English

The first Marstek Monitor release for macOS. The Windows version works as before.

- macOS 12 or later, one app for Apple Silicon and Intel. To install, open `marstek-monitor-1.1.0-macos.dmg` and drag the app into Applications.
- The app is signed and notarized by Apple. On the first launch macOS asks once to confirm opening it, as it does for any app downloaded from the Internet.
- The same tabs and notifications as on Windows. The icon lives in the menu bar and shows whether the power is on; the Dock icon appears only while the window is open.
- Clicking a notification opens the window on the matching tab.
- "Open at login" in Settings. Data is kept in `~/Library/Application Support/MarstekMonitor`.
- The app asks for Local Network access and, if it is missing, shows where to turn it on.
- One-click update on the Mac too: download, SHA-256 and developer signature check, database backup, app swap and restart. It works when the app is in Applications; otherwise "Download" is offered.
- Releases are now built on GitHub, and each one carries files for both systems.
- Windows: no visible change. The update check takes only the Windows installer from a release, not the Mac file.

## 1.0.0

### Українською

Перший публічний випуск Marstek Monitor для Windows: неофіційний додаток у треї, що стежить за станцією Marstek Venus E 3.0 (прошивка v151) по локальній мережі та веде облік відключень світла.

- Вкладки «Огляд», «Відключення», «Помилки», «Батарея», «Енергія»: заряд, потужність, енергія, цикли батареї з оцінкою здоров'я, журнал подій.
- Іконка в треї та сповіщення Windows: світло зникло / з'явилось, низький заряд, станція заряджена, немає зв'язку. Тиха година і режим «Не турбувати».
- Адаптивне опитування станції; «Помилки» мають посторінковий перегляд.
- У журналі видно короткі обриви зв'язку зі станцією (час і тривалість).
- Інсталятор для поточного користувача: без прав адміністратора, без правил брандмауера. Видалення зберігає ваші дані в `%LOCALAPPDATA%\MarstekMonitor`.
- Перевірка оновлень: раз на добу (можна вимкнути) і кнопкою. Оновлення ставиться лише після вашого кліку: завантаження, перевірка SHA-256, резервна копія бази, тиха інсталяція.
- Експорт у CSV, запуск разом з Windows, інтерфейс українською та англійською.
- Лише локальна мережа, без телеметрії. Станції надсилаються тільки `ES.GetStatus` і `Marstek.GetDevice`; `Bat.GetStatus` не викликається свідомо.
- Інсталятор не підписаний, тому SmartScreen попередить про невідомого видавця.

### English

First public release of Marstek Monitor for Windows: an unofficial tray app that watches a Marstek Venus E 3.0 (firmware v151) over the local network and keeps a log of power outages.

- Tabs Overview, Outages, Errors, Battery and Energy: charge, power, energy, battery cycles with a health estimate, an event journal.
- Tray icon and Windows notifications: power lost / restored, low charge, station charged, no connection. Quiet hours and Do Not Disturb.
- Adaptive polling of the station; the Errors tab is paged.
- The journal shows short connection drops to the station (time and duration).
- Per-user installer: no administrator rights, no firewall rule. Uninstalling keeps your data in `%LOCALAPPDATA%\MarstekMonitor`.
- Update check: once a day (can be turned off) and by button. An update is installed only after you click: download, SHA-256 check, database backup, silent install.
- CSV export, start with Windows, Ukrainian and English UI.
- Local network only, no telemetry. Only `ES.GetStatus` and `Marstek.GetDevice` are sent to the station; `Bat.GetStatus` is deliberately never called.
- The installer is unsigned, so SmartScreen will warn about an unknown publisher.
