# Changelog

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
