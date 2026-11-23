# Marstek Monitor — каталог текстів (uk / en)

Дата: 2026-11-21 · Статус: чернетка на погодження · Автор: UX writer
Джерела: `2026-11-21-marstek-monitor-design.md` (§5 — погоджений UX), макети `…-mockups/01`, `02` (02 замінює ранні блоки 01).

Позначення: **[рішення власника]** — треба підтвердити (є рекомендований варіант за замовчуванням); **[placeholder]** — приклад, не реальні дані.

Ключі — `розділ.елемент.стан`. Ключі описують зміст, а не формулювання, тож текст можна міняти без перейменування ключа. Аргументи — тільки іменовані (`{soc}`, `{time}`); числа, дати й тривалості форматує frontend (див. §9), у рядок вони приходять уже готовими, крім випадків із `plural`.

---

## 1. Голос і тон

1. **Спокійно і точно.** Текст читають під час відключення, часто з тривогою. Факт, час, число, і що буде далі. Без окличних знаків, без жартів, без «Ой!».
2. **Спочатку головне.** Найважливіше — перші 2–3 слова: «Зникло світло», «Низький заряд — 20%». Подробиці — після.
3. **Чесно про те, чого ми не знаємо.** Застарілі дані позначаються як застарілі, пропуски називаються пропусками, прогноз має знак «≈». Ми ніколи не пишемо «зараз» про дані, яким 10 хвилин.
4. **Звертання на «ви», кнопки — інфінітивом** («Додати», «Увійти», «Експортувати»). Так уже в макетах («Перевірте вентиляцію», «Не бачите своєї?»).
5. **Слова власника, не інженера.** «Світло», «станція», «заряд», а не «grid input», «SOC», «ESS». Технічні назви (LAN, API, IP) — тільки там, де людина має щось налаштувати.
6. **Без провини користувача.** Збій зв'язку чи API — не «ви не підключились», а «станція не відповідає».
7. **Помилка = що сталося → що це означає → що робити.** Порада — одна, конкретна, виконувана.
8. **Регістр речень скрізь** (у uk та en): «Додати станцію», «Add station», не «Add Station».
9. **ⓘ — лише там, де значення можна прочитати хибно** («≥», «≈», «є пропуски в даних», «невідомо», оцінки й пороги), а не біля кожного підпису. Пояснення каже, що це означає, і, якщо треба, що робити або що все гаразд (§15).

## 2. Глосарій (контрольований словник)

| Поняття | UK (UI) | EN (UI) | У коді | Не вживати | Примітка |
|---|---|---|---|---|---|
| Наявність живлення від електромережі (стан / подія) | **світло** («світло є», «без світла», «зникло світло») | **grid** («grid on», «grid off», «power outage») | `grid_present` | «мережа» у значенні стану, «електромережа», «енергопостачання» | Слово власника і всієї країни. «Електромережа» — лише в README/«Про програму» |
| Період без світла | **відключення** (скор. «відкл.») | **outage** | `outage` | «аварія», «блекаут», «знеструмлення» | «Відключення» не залежить від причини (графік чи аварія) — ми її не знаємо |
| Електромережа як **джерело енергії** в обліку | **мережа** («з мережі», «Мережа / сонце») | **grid** («from grid») | `grid` | «світло» у цьому значенні | Лише на вкладці «Енергія», поруч із «сонце». Ніде більше «мережа» без уточнення |
| Домашня Wi-Fi/LAN-мережа | **локальна мережа**, у кнопках — «у вашому Wi-Fi»; коротко **LAN** у пігулках | **local network**, **LAN** | `lan` | «мережа» без «локальна» | Бо «мережа» в цьому продукті вже означає електромережу |
| Пристрій | **станція** (вперше в онбордингу й «Про програму» — «зарядна станція») | **station** | `station` | «батарея» (про пристрій), «павербанк», «ДБЖ», «інвертор» | «Станція» — слово власника; «батарея» зайнята вкладкою про здоров'я АКБ |
| Акумулятор усередині станції | **батарея** | **battery** | `battery` | «акумулятор», «АКБ» в UI | Вкладка «Батарея» = ємність, цикли, температура |
| Рівень заряду, % | **заряд** («заряд 72%») | **charge** («charge 72%») | `soc` | «рівень заряду», «SOC», «ємність» (це інше) | «Рівень заряду» допустимо тільки в доступній назві (a11y) |
| Процес заряджання / розряджання | **заряджається / розряджається** (стан); **заряд / розряд** — *тільки* одразу після значення у Вт («420 Вт · розряд») | **charging / discharging** | `power > 0 / < 0` | «зарядка», «розрядка» | Одиниця Вт знімає двозначність із «заряд 72%» |
| Потужність ≈ 0 | **очікування** | **idle** | `idle` | «простій», «standby», «сон» | |
| Станція вимкнула вихід через низький заряд | **станція, ймовірно, розрядилась** («станція, ймовірно, розрядилась ≈ о 00:55») | **station probably ran out** («probably ran out at ≈ 00:55») | `station_died_at` | «померла», «вимкнулась» (у різних місцях різне) | Одне формулювання в таблиці, тултіпі, журналі |
| Нижня межа заряду для прогнозу | **нижня межа** («до 10%») | **floor** («to 10%») | `soc_floor` | «мінімум», «відсічка» | |
| Час до нижньої межі | **До розряду** (кільце) | **Until empty** | `eta_empty` | | Погоджена назва кільця |
| Час до 100% | **До повного** | **Until full** | `eta_full` | | |
| Час роботи в очікуванні при типовому навантаженні | **Вистачить на** | **Would last** | `runtime_idle` | «автономність» | |
| Звідки дані | **джерело даних**: LAN · хмара · Bluetooth | **data source**: LAN · cloud · Bluetooth | `source` | «канал», «транспорт» | |
| Хмара Marstek | **хмара Marstek**, **акаунт Marstek** | **Marstek cloud**, **Marstek account** | `cloud` | «сервер», «облік» | |
| Дані старші за поріг | **дані застарілі** | **data is out of date** | `stale` | «неактуальні», «старі» | |
| Проміжок без даних | **немає даних** / **пропуск у даних** | **no data** / **gap in data** | `gap` | замальовування як «світло є/немає» | |
| Частина відключення відновлена з хмари | **частково з хмари** | **partly from cloud** | `partial_cloud` | | |
| Рівні подій | **інфо · попередження · помилка** | **info · warning · error** | `level` | «критична», «фатальна» | |
| Локальний API Marstek | **локальний API** | **local API** | `open_api` | «Open API» в UI | Технічний термін, лише в підказках з налаштування |
| Здоров'я батареї | **здоров'я** (% фактичної ємності від номінальної) | **health** | `health` | «знос», «SOH» | |
| Цикл | **цикл** | **cycle** | `cycles` | | |
| Тиха година | **тиха година** | **quiet hours** | `quiet_hours` | «не турбувати» | |

Назва моделі (Venus E 3.0) і «Marstek» не перекладаються. Назва станції — те, що задав користувач («Дім», «Дача»), ніколи не перекладається і не відмінюється (тому конструкції будуються так, щоб назва стояла в називному відмінку: «Marstek Monitor · Дім»).

---

## 3. Каркас: вкладки, перемикач станції, загальні стани

| Ключ | UK | EN | Бюджет | Примітка |
|---|---|---|---|---|
| `nav.tab.overview` | Огляд | Overview | ≤ 12 | |
| `nav.tab.outages` | Відключення | Outages | ≤ 12 | |
| `nav.tab.errors` | Помилки | Errors | ≤ 12 | |
| `nav.tab.battery` | Батарея | Battery | ≤ 12 | |
| `nav.tab.energy` | Енергія | Energy | ≤ 12 | |
| `nav.settings.a11y` | Налаштування | Settings | — | Доступна назва кнопки ⚙ |
| `nav.stationSwitcher.a11y` | Вибрати станцію. Зараз: {station} | Choose station. Current: {station} | — | |
| `common.period.today` | Сьогодні | Today | ≤ 10 | Перемикач періоду (Відключення, Енергія) |
| `common.period.7d` | 7 днів | 7 days | ≤ 10 | |
| `common.period.30d` | 30 днів | 30 days | ≤ 10 | |
| `common.period.custom` | Свій період | Custom | ≤ 12 | Відкриває календар; після вибору показує діапазон `{range}` |
| `common.period.custom.a11y` | Вибрати період у календарі | Pick a date range | — | |
| `common.chart.range.24h` | 24 год | 24 h | ≤ 7 | Графік заряду на Огляді |
| `common.chart.range.7d` | 7 днів | 7 days | ≤ 7 | |
| `common.chart.range.30d` | 30 днів | 30 days | ≤ 7 | |
| `common.value.none` | — | — | 1 | Значення невідоме / недоступне |
| `common.loading` | Завантажуємо… | Loading… | — | Тільки якщо > 1 с |
| `common.action.retry` | Спробувати ще раз | Try again | ≤ 18 | |
| `common.action.cancel` | Скасувати | Cancel | ≤ 12 | |
| `common.action.close` | Закрити | Close | ≤ 12 | |
| `common.action.save` | Зберегти | Save | ≤ 12 | |
| `common.action.howTo` | Як це зробити | How to do this | ≤ 18 | Посилання на підказку |
| `common.estimate.prefix` | ≈ | ≈ | 1 | Перед кожним прогнозом |

### 3.1 Застарілі дані й пропуски (усі екрани)

Правило: якщо найсвіжіші дані старші за поріг (§7.2 спеки), значення на екрані сірішають, а над ними — банер. Ми не показуємо застарілі дані як поточні.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `stale.banner.today` | Дані застарілі — останні о {time} | Data is out of date — last update at {time} | Банер над кільцями, постійний, поки не оновиться |
| `stale.banner.otherDay` | Дані застарілі — останні {date} о {time} | Data is out of date — last update on {date} at {time} | Якщо не сьогодні |
| `stale.banner.reason.reconnecting` | Станція не відповідає. Пробуємо знову… | The station isn't responding. Retrying… | Другий рядок банера |
| `stale.banner.reason.cloudOnly` | Локально станція не відповідає, хмара оновлюється рідше. | No local connection; the cloud updates less often. | Коли лишилась тільки хмара і її дані теж старші за поріг |
| `stale.ring.a11ySuffix` | (застаріле значення, станом на {time}) | (out of date, as of {time}) | Додається до доступної назви кожного кільця |
| `stale.recovered.announce` | Дані знову оновлюються. | Data is updating again. | Live region, polite |
| `gap.label` | немає даних | no data | Пунктирна рамка на графіках |
| `gap.label.appOff` | додаток не працював | app wasn't running | Коли знаємо, що причина — вимкнений ПК / додаток |
| `gap.tooltip` | Немає даних з {start} до {end}. Що було зі світлом у цей час, невідомо. | No data from {start} to {end}. Grid status during this time is unknown. | |
| `gap.tooltip.appOff` | Додаток не працював з {start} до {end}, а хмара цей час не віддала. Що було зі світлом, невідомо. | The app wasn't running from {start} to {end}, and the cloud had no data for it. Grid status is unknown. | |

---

## 4. Огляд

### 4.1 Кільця — три стани

Бюджети: підпис кільця (`label`) ≤ 14 символів; центр ≤ 5; рядок одиниць у центрі (`unit`) ≤ 15; підпис під кільцем (`sub`) ≤ 22 (uk — найдовша мова, бюджет рахується на неї).

**Кільце 1 — Заряд** (однакове в усіх станах)

| Ключ | UK | EN | Приклад uk |
|---|---|---|---|
| `overview.ring.soc.label` | Заряд | Charge | |
| `overview.ring.soc.value` | {soc}% | {soc}% | 72% |
| `overview.ring.soc.sub` | {energy} з {capacity} кВт·год | {energy} of {capacity} kWh | 3,7 з 5,12 кВт·год (18) |
| `overview.ring.soc.a11y` | Рівень заряду {soc}%, {energy} з {capacity} кіловат-годин | Charge {soc}%, {energy} of {capacity} kilowatt-hours | |

**Кільце 2 — Потужність**

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `overview.ring.power.label` | Потужність | Power | |
| `overview.ring.power.value` | {watts} | {watts} | Абсолютне значення, без знака |
| `overview.ring.power.unit.discharging` | Вт · розряд | W · discharging | Рядок у центрі. «розряд» дозволено лише після Вт (глосарій) |
| `overview.ring.power.unit.charging` | Вт · заряд | W · charging | |
| `overview.ring.power.unit.idle` | Вт · очікування | W · idle | |
| `overview.ring.power.sub.discharging` | з {max} Вт макс. | of {max} W max | макс. розряду моделі |
| `overview.ring.power.sub.charging` | з {max} Вт макс. | of {max} W max | макс. заряду моделі |
| `overview.ring.power.sub.idle` | ні заряду, ні розряду | no power in or out | 21 / 18 |
| `overview.ring.power.a11y` | {state, select, discharging {Розряджається, {watts} Вт з {max} максимальних} charging {Заряджається, {watts} Вт з {max} максимальних} other {Очікування, 0 Вт}} | {state, select, discharging {Discharging at {watts} W of {max} max} charging {Charging at {watts} W of {max} max} other {Idle, 0 W}} | |

**Кільце 3 — прогноз (залежить від стану)**

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `overview.ring.toEmpty.label` | До розряду | Until empty | Стан «розряд» |
| `overview.ring.toFull.label` | До повного | Until full | Стан «заряд» |
| `overview.ring.lasts.label` | Вистачить на | Would last | Стан «очікування» |
| `overview.ring.eta.value.hm` | {h}:{mm} | {h}:{mm} | < 24 год: «7:30». `mm` — з нулем попереду |
| `overview.ring.eta.unit.hm` | год : хв | h : min | |
| `overview.ring.eta.value.h` | {hours} | {hours} | ≥ 24 год: ціле число годин, «38» |
| `overview.ring.eta.unit.h` | год | hours | |
| `overview.ring.toEmpty.sub` | до {floor}% · ≈ о {time} | to {floor}% · ≈ {time} | «до 10% · ≈ о 22:45» (18) |
| `overview.ring.toEmpty.sub.otherDay` | до {floor}% · ≈ {weekday} {time} | to {floor}% · ≈ {weekday} {time} | Якщо не сьогодні: «до 10% · ≈ пт 03:10» |
| `overview.ring.toFull.sub` | до 100% · ≈ о {time} | to 100% · ≈ {time} | |
| `overview.ring.lasts.sub` | при сер. {watts} Вт у відкл. | at avg outage {watts} W | 27 / 23 при 3-значних Вт — на межі; перевірити в рендері, запасний uk: «при {watts} Вт у відкл.» |
| `overview.ring.eta.noData.value` | — | — | |
| `overview.ring.lasts.noData.sub` | ще немає даних | not enough data yet | Немає історії відключень за 30 днів (§7.4) |
| `overview.ring.toEmpty.noData.sub` | рахуємо… | calculating… | Перші ~5 хв, поки немає ковзного середнього |
| `overview.ring.toEmpty.a11y` | До нижньої межі {floor}% приблизно {duration}, близько {time} | About {duration} until {floor}%, around {time} | `{duration}` — довга форма (§9.2) |
| `overview.ring.toFull.a11y` | До повного заряду приблизно {duration}, близько {time} | About {duration} until full, around {time} | |
| `overview.ring.lasts.a11y` | При середньому навантаженні {watts} Вт заряду вистачить приблизно на {duration} | At the average outage load of {watts} W, the charge would last about {duration} | |

**Кільце 4 — Без світла сьогодні** — рішення власника 2026-11-21: показуємо час без світла, а не відсоток. Відсотків на цьому кільці немає. Ключі перейменовано з `gridToday` на `offToday`. Старі ключі `overview.ring.gridToday.*` видалено.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `overview.ring.offToday.label` | Без світла сьогодні | Grid off today | 19 / 14. Довше за бюджет 14: підпис кільця переноситься у 2 рядки (hi-fi README §5), тож допустимо. Перевірити в рендері |
| `overview.ring.offToday.value` | {h}:{mm} | {h}:{mm} | Центр, одне велике значення в годинниковій формі — як у «До розряду». «5:27»; менше години — «0:27»; без відключень — «0:00». `mm` — з нулем попереду. Сьогоднішня доба ≤ 24 год, тож формат «д» не потрібен |
| `overview.ring.offToday.unit` | год : хв | h : min | Рядок під значенням (= `overview.ring.eta.unit.hm`) |
| `overview.ring.offToday.value.atLeast` | ≥ {value} | ≥ {value} | Тривалість — нижня межа (STALE / NO_LINK / LIKELY_DEAD, пропуски сьогодні): «≥ 1:20». Бюджет центру 5 символів → з «≥» до 7; перевірити в рендері |
| `overview.ring.offToday.sub` | {count, plural, one {# відключення} few {# відключення} many {# відключень} other {# відключення}} | {count, plural, one {# outage} other {# outages}} | «2 відключення», «5 відключень». Включає відключення, що триває |
| `overview.ring.offToday.sub.ongoing` | {count, plural, one {# відключення, триває} few {# відключення, одне триває} many {# відключень, одне триває} other {# відключення, одне триває}} | {count, plural, one {# outage, ongoing} other {# outages, one ongoing}} | Необов'язковий варіант. Рейка й банер уже кажуть, що світла немає; вмикати, лише якщо дизайнер хоче. До 22 символів: «12 відключень, одне триває» = 26, тож для ≥ 10 — звичайний `sub` |
| `overview.ring.offToday.sub.none` | без відключень | no outages | |
| `overview.ring.offToday.sub.gaps` | є пропуски в даних | some data missing | Замість `sub`, якщо сьогодні є пропуски > 15 хв. Центр тоді з префіксом «≥» |
| `overview.ring.offToday.a11y` | Сьогодні без світла {duration}. {count, plural, one {# відключення} few {# відключення} many {# відключень} other {# відключення}}. | Grid off today: {duration}. {count, plural, one {# outage} other {# outages}}. | `{duration}` — довга форма (§9.2); якщо нижня межа — «щонайменше {duration}» / «at least {duration}» |

Приклади: «5:27 · год : хв · 2 відключення»; «0:27 · 1 відключення»; «0:00 · без відключень»; «≥ 1:20 · 1 відключення». Доступна назва читає тривалість довгою формою, не цифрами: «Сьогодні без світла 5 годин 27 хвилин. 2 відключення.» (en: "Grid off today: 5 hours 27 minutes. 2 outages."). Для «0:00» — `duration.long.m` з 0: «0 хвилин» / "0 minutes".

### 4.2 Графік заряду станції

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `overview.chart.title` | Заряд станції, % | Station charge, % | |
| `overview.chart.band.outage` | без світла {duration} | grid off {duration} | Підпис над сірою смугою, коротка тривалість |
| `overview.chart.band.outage.ongoing` | без світла {duration}, триває | grid off {duration}, ongoing | |
| `overview.chart.tooltip.point` | {time} · {soc}% · {stateText} | {time} · {soc}% · {stateText} | `stateText` = `state.*` (§4.4) |
| `overview.chart.tooltip.band` | Без світла {start}–{end} · {duration} | Grid off {start}–{end} · {duration} | |
| `overview.chart.empty` | Графік з'явиться, щойно надійдуть перші дані. | The chart will appear once the first data arrives. | Нова станція |
| `overview.chart.a11y` | Графік заряду станції за {range}. Відключень: {count}. | Station charge chart for {range}. Outages: {count}. | |

### 4.3 Права колонка

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `overview.side.grid.label` | Світло | Grid | Макет мав «Мережа» — див. §14, п. 3 |
| `overview.side.grid.on` | є | on | |
| `overview.side.grid.onSince` | є з {time} · {duration} | on since {time} · {duration} | Якщо сьогодні вже було відключення |
| `overview.side.grid.off` | немає з {time} · {duration} | off since {time} · {duration} | «немає з 14:05 · 1 год 12 хв» |
| `overview.side.grid.offApprox` | немає з ≈ {time} · {duration} | off since ≈ {time} · {duration} | Початок відомий неточно (зникло, поки додаток не працював) |
| `overview.side.grid.unknown` | невідомо | unknown | Немає свіжих даних |
| `overview.side.prevOutage.label` | Попереднє відключення | Previous outage | |
| `overview.side.prevOutage.value` | {date} {start}–{end} · {duration} | {date} {start}–{end} · {duration} | «вчора 19:30–01:40 · 6 год 10 хв» |
| `overview.side.prevOutage.none` | ще не було | none yet | |
| `overview.side.errors.label` | Помилки | Errors | |
| `overview.side.errors.value` | {count, plural, =0 {нових немає} one {# нова} few {# нові} many {# нових} other {# нової}} | {count, plural, =0 {no new ones} one {# new} other {# new}} | Клік → вкладка Помилки |
| `overview.side.battery.label` | Батарея | Battery | |
| `overview.side.battery.value` | {cycles, plural, one {# цикл} few {# цикли} many {# циклів} other {# циклу}} · {health}% ємності | {cycles, plural, one {# cycle} other {# cycles}} · {health}% capacity | |
| `overview.side.battery.noHealth` | {cycles, plural, one {# цикл} few {# цикли} many {# циклів} other {# циклу}} · ємність рахуємо | {cycles, plural, one {# cycle} other {# cycles}} · capacity pending | |
| `overview.side.source.label` | Джерело даних | Data source | |
| `overview.side.source.value` | {source} · {age} | {source} · {age} | «LAN · 3 с тому», `source` = `source.*` (§8) |

### 4.4 Стани станції (пігулки, тултіпи, трей)

| Ключ | UK | EN |
|---|---|---|
| `state.discharging` | розряджається | discharging |
| `state.charging` | заряджається | charging |
| `state.idle` | очікування | idle |
| `state.withPower.discharging` | розряд {watts} Вт | discharging {watts} W |
| `state.withPower.charging` | заряд {watts} Вт | charging {watts} W |
| `state.grid.on` | світло є | grid on |
| `state.grid.off` | без світла {duration} | grid off {duration} |
| `state.grid.unknown` | світло: невідомо | grid: unknown |
| `state.stationOut` | станція, ймовірно, розрядилась | station probably ran out |

---

## 5. Відключення

### 5.1 Кільця і статистика

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `outages.ring.grid.label` | Світло | Grid | Період видно з перемикача — не дублюємо в підписі (бюджет) |
| `outages.ring.grid.value` | {percent}% | {percent}% | |
| `outages.ring.grid.sub` | {duration} без світла | {duration} off | «39 год без світла» |
| `outages.ring.coverage.label` | Покриття | Coverage | Макет: «Покриття відкл.» |
| `outages.ring.coverage.value` | {percent}% | {percent}% | % відключень, які станція протягнула до кінця |
| `outages.ring.coverage.sub` | {count, plural, =0 {усі протягнула} one {# раз, ймовірно, розрядилась} few {# рази, ймовірно, розрядилась} many {# разів, ймовірно, розрядилась} other {# рази, ймовірно, розрядилась}} | {count, plural, =0 {lasted through all} one {probably ran out # time} other {probably ran out # times}} | ≤ 22 — «# раз, ймовірно, розрядилась» понад бюджет 22 — переноситься у 2 рядки. Макет: «1 раз розрядилась раніше» (23) — «раніше» знято за бюджетом |
| `outages.ring.coverage.a11y` | Станція протягнула {percent}% відключень до кінця. Розрядилась раніше: {count}. | The station lasted through {percent}% of outages. Ran out early: {count}. | |
| `outages.stat.count` | Відключень: {count} | Outages: {count} | |
| `outages.stat.average` | Середнє: {duration} | Average: {duration} | |
| `outages.stat.longest` | Найдовше: {duration} ({date}) | Longest: {duration} ({date}) | «6 год 10 хв (вт, 18.11)» |

### 5.2 Графік «Без світла по днях»

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `outages.chart.title.days` | Без світла по днях | Grid off by day | |
| `outages.chart.title.weeks` | Без світла по тижнях | Grid off by week | Період > 30 днів |
| `outages.chart.axis.y` | години без світла | hours off | |
| `outages.chart.axis.tick` | {hours} год | {hours} h | |
| `outages.chart.axis.day` | {weekday} {date} | {weekday} {date} | «пт 15.11» / «Fri 15» |
| `outages.chart.axis.week` | {start}–{end} | {start}–{end} | «11–17.11» |
| `outages.tooltip.title` | {weekdayLong}, {dateLong} | {weekdayLong}, {dateLong} | «Пт, 15 листопада» (перша літера велика — початок речення) |
| `outages.tooltip.title.today` | {weekdayLong}, {dateLong} (сьогодні) | {weekdayLong}, {dateLong} (today) | |
| `outages.tooltip.title.week` | Тиждень {start} – {end} | Week of {start} – {end} | |
| `outages.tooltip.count` | Відключень: {count} | Outages: {count} | |
| `outages.tooltip.count.ongoing` | Відключень: {count} (одне триває) | Outages: {count} (one ongoing) | |
| `outages.tooltip.total` | Без світла: {duration} ({percent}% доби) | Grid off: {duration} ({percent}% of the day) | % доби — завжди, макет показує його вибірково; для тижня — «тижня» / «of the week» (`outages.tooltip.total.week`) |
| `outages.tooltip.total.week` | Без світла: {duration} ({percent}% тижня) | Grid off: {duration} ({percent}% of the week) | |
| `outages.tooltip.longest` | Найдовше: {duration} ({start}–{end}) | Longest: {duration} ({start}–{end}) | Лише якщо відключень > 1 |
| `outages.tooltip.minSoc` | Мін. заряд: {soc}% | Lowest charge: {soc}% | |
| `outages.tooltip.minSoc.out` | Мін. заряд: {soc}% — станція, ймовірно, розрядилась | Lowest charge: {soc}% — station probably ran out | Червоним |
| `outages.tooltip.partialCloud` | ⋯ частково з хмари | ⋯ partly from cloud | |
| `outages.tooltip.gaps` | Є пропуски в даних: {duration} | Data missing for {duration} | |
| `outages.tooltip.none` | Відключень не було | No outages | |
| `outages.tooltip.hint` | Клацніть, щоб показати ці відключення в таблиці | Click to show these outages in the table | |
| `outages.chart.filterActive` | Показано відключення за {date} · Показати всі | Showing outages on {date} · Show all | Над таблицею після кліку |

### 5.3 Таблиця подій

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `outages.table.col.start` | Початок | Start | |
| `outages.table.col.end` | Кінець | End | |
| `outages.table.col.duration` | Тривалість | Duration | |
| `outages.table.col.soc` | Заряд на початку → в кінці | Charge at start → end | |
| `outages.table.col.note` | Примітка | Note | |
| `outages.table.end.ongoing` | триває | ongoing | |
| `outages.table.start.approx` | ≈ {time} | ≈ {time} | Початок невідомий точно |
| `outages.note.stationOut` | станція, ймовірно, розрядилась ≈ о {time} | station probably ran out at ≈ {time} | |
| `outages.note.partialCloud` | частково з хмари | partly from cloud | |
| `outages.note.gap` | є пропуск у даних | gap in data | Тривалість може бути неточною |
| `outages.note.startedWhileOff` | почалось, поки додаток не працював | started while the app was off | Разом із «≈» на початку |
| `outages.table.soc.unknown` | — | — | Заряд невідомий |

### 5.4 Порожні стани

| Ключ | UK | EN | Вид |
|---|---|---|---|
| `outages.empty.firstUse.title` | Відключень ще не записано | No outages recorded yet | Перше використання |
| `outages.empty.firstUse.body` | Додаток рахує відключення з моменту, як підключив станцію. Поки комп'ютер вимкнений, дані можна дозавантажити лише з хмари Marstek. | The app records outages from the moment the station is connected. While this PC is off, data can only be filled in from the Marstek cloud. | |
| `outages.empty.firstUse.action` | Підключити хмару | Connect cloud | Лише якщо хмара не підключена |
| `outages.empty.period` | За цей період відключень не було. | No outages in this period. | Є історія, але не в цьому періоді |
| `outages.empty.periodWithGaps` | Записаних відключень немає, але за {duration} даних немає — відключення могли бути. | No outages recorded, but {duration} of data is missing — there may have been some. | Чесно про пропуски |

---

## 6. Помилки

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `errors.table.col.when` | Коли | When | |
| `errors.table.col.level` | Рівень | Level | |
| `errors.table.col.what` | Що сталося | What happened | |
| `errors.table.col.todo` | Що робити | What to do | |
| `errors.level.info` | інфо | info | ≤ 12 (бейдж) |
| `errors.level.warning` | попередження | warning | 12 — межа бейджа |
| `errors.level.error` | помилка | error | |
| `errors.filter.label` | Показати: | Show: | |
| `errors.filter.all` | усі | all | |
| `errors.filter.errors` | помилки | errors | |
| `errors.filter.warnings` | попередження | warnings | |
| `errors.filter.connection` | зв'язок | connection | |
| `errors.what.withCode` | {title} (код {code}) | {title} (code {code}) | |
| `errors.unknown.title` | Невідома помилка (код {code}) | Unknown error (code {code}) | §7.7 спеки |
| `errors.unknown.todo` | Ми ще не знаємо цього коду. Якщо помилка повторюється, зверніться в підтримку Marstek і назвіть код. | We don't know this code yet. If it keeps happening, contact Marstek support and give them the code. | |
| `errors.empty.all` | За цей час станція не повідомляла про помилки. | The station reported no errors in this period. | |
| `errors.empty.filtered` | Немає подій цього типу. Показати всі | No events of this type. Show all | |
| `errors.empty.noLogSource` | Станція не передає журнал помилок через {source}. Тут поки лише події зв'язку додатка. | The station doesn't send its error log over {source}. Only the app's connection events are shown for now. | Якщо джерело не віддає коди (§9 п. 3) |
| `errors.new.badge.a11y` | {count, plural, one {# нова подія} few {# нові події} many {# нових подій} other {# нової події}} | {count, plural, one {# new event} other {# new events}} | |

### 6.1 Події зв'язку (пише сам додаток)

| Ключ | Рівень | UK — що сталося | UK — що робити | EN — what happened | EN — what to do |
|---|---|---|---|---|---|
| `conn.event.lanLost` | інфо | Втрачено зв'язок по локальній мережі | Додаток перемкнувся на {source}. Нічого робити не треба. | Lost local network connection | Switched to {source}. Nothing to do. |
| `conn.event.lanLost.noFallback` | попередження | Втрачено зв'язок по локальній мережі | Перевірте, чи станція й цей комп'ютер у тому самому Wi-Fi. | Lost local network connection | Check that the station and this PC are on the same Wi-Fi. |
| `conn.event.lanRestored` | інфо | Зв'язок по локальній мережі відновлено (не було {duration}) | — | Local connection restored (lost for {duration}) | — |
| `conn.event.ipChanged` | інфо | Станція змінила адресу: тепер {ip} | — | Station address changed to {ip} | — |
| `conn.event.cloudLost` | інфо | Хмара Marstek не відповідає | Дані йдуть локально. Якщо світла немає, інтернет теж може зникнути. | Marstek cloud isn't responding | Data still comes over the local network. |
| `conn.event.cloudRestored` | інфо | Хмара Marstek знову відповідає | — | Marstek cloud is responding again | — |
| `conn.event.cloudSessionExpired` | попередження | Сеанс у хмарі Marstek завершився | Увійдіть знову в Налаштування → Підключення. | Marstek cloud session expired | Sign in again in Settings → Connection. |
| `conn.event.noConnection` | попередження | Немає зв'язку зі станцією — ні локально, ні через хмару | Перевірте, чи станція ввімкнена і чи працює роутер. | No connection to the station — neither local nor cloud | Check that the station is on and the router is working. |
| `conn.event.backfilled` | інфо | Дані за {duration}, поки додаток не працював, дозавантажено з хмари | — | Filled in {duration} of data from the cloud while the app was off | — |
| `conn.event.backfillPartial` | інфо | Хмара віддала дані лише частково — за {duration} даних немає | — | The cloud returned only part of the data — {duration} is missing | — |
| `conn.event.apiSilent` | попередження | Локальний API станції перестав відповідати | Це відома проблема деяких прошивок. Додаток спробує знову; допоможе перезапуск станції. | The station's local API stopped responding | Known issue on some firmware. The app keeps retrying; restarting the station helps. |

### 6.2 Таблиця кодів помилок станції — структура

Реальні коди ще невідомі (§9 спеки, п. 3). Нижче — формат таблиці, яку заповнює розробник, і приклади. **Усі рядки — [placeholder]**: коди, тексти й поради вигадані для демонстрації формату і не повинні потрапити в реліз.

Ключі: `stationError.{code}.title`, `stationError.{code}.meaning`, `stationError.{code}.todo`, плюс `level` (інфо / попередження / помилка) і `source` (звідки код: LAN / хмара).

Правила наповнення:
- `title` ≤ 40 символів uk (іде в заголовок сповіщення після «Помилка: »).
- `meaning` — одне речення, що це означає для людини, без внутрішніх назв модулів.
- `todo` — одна конкретна дія. Якщо дії немає — «Нічого робити не треба» і чому. Якщо треба в підтримку — так і сказати.
- Порада ніколи не просить розбирати станцію чи лізти всередину.
- Джерело кожного рядка (документація Marstek, спільнота, власне спостереження) — у колонці «Звідки», щоб перевірити.

| Код | Рівень | Title uk / en | Meaning uk / en | Todo uk / en | Звідки |
|---|---|---|---|---|---|
| `E07` **[placeholder]** | помилка | Перегрів інвертора / Inverter overheating | Станція обмежила або вимкнула вихід, щоб охолонути. / The station limited or cut output to cool down. | Перевірте вентиляцію: не ставте впритул до стіни й не накривайте. / Check ventilation: keep it away from the wall and uncovered. | макет 01 |
| `W10` **[placeholder]** | попередження | Низький заряд — вихід вимкнено / Low charge — output off | Спрацював захист від глибокого розряду на {floor}%. / Deep-discharge protection kicked in at {floor}%. | Нічого робити не треба: станція ввімкне вихід, коли з'явиться світло. / Nothing to do: output turns back on when the grid returns. | макет 01 |
| `W21` **[placeholder]** | попередження | Батарея надто холодна / Battery too cold | Нижче 0 °C станція не заряджається, щоб берегти батарею. / Below 0 °C the station won't charge, to protect the battery. | Перенесіть станцію в тепліше місце. / Move the station somewhere warmer. | — |
| `E30` **[placeholder]** | помилка | Збій зв'язку з батареєю / Battery communication fault | Інвертор не отримує даних від батареї. / The inverter isn't getting data from the battery. | Перезапустіть станцію. Якщо повторюється — зверніться в підтримку Marstek з кодом E30. / Restart the station. If it repeats, contact Marstek support with code E30. | — |

---

## 7. Батарея

> **v1 (рішення власника 2026-11-23, Ruling 57):** додаток не викликає `Bat.GetStatus` — на деяких прошивках (і тричі на v151 власника) після нього станція замовкає або перезапускається, а це може знеструмити будинок (АВР). Тож рядки `battery.ring.temp.*` і `battery.hidden.note` нижче в v1 не показуються (і їх немає в каталогах), вкладка «Батарея» — два кільця: здоров'я й цикли. Деталі — `docs/dev-notes.md`.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `battery.ring.health.label` | Здоров'я | Health | |
| `battery.ring.health.value` | {percent}% | {percent}% | |
| `battery.ring.health.sub` | {actual} з {nominal} кВт·год | {actual} of {nominal} kWh | |
| `battery.ring.health.noData.sub` | ще рахуємо | still estimating | Центр «—» |
| `battery.ring.cycles.label` | Ресурс циклів | Cycle life | |
| `battery.ring.cycles.value` | {cycles} | {cycles} | |
| `battery.ring.cycles.sub` | з понад {rated} при {dod}% DoD | of {rated}+ at {dod}% DoD | Ресурс — паспортний (Ruling 19), без «~» |
| `battery.ring.cycles.sub.estimated` | з понад {rated} при {dod}% DoD · оцінка | of {rated}+ at {dod}% DoD · estimate | Коли API не віддає цикли і ми рахуємо самі (§7.5); саме число циклів — з «≈ » |
| `battery.ring.temp.label` | Температура | Temperature | |
| `battery.ring.temp.value` | {temp}° | {temp}° | |
| `battery.ring.temp.sub` | норма {min}…{max} °C | normal {min}…{max} °C | |
| `battery.ring.temp.sub.high` | вище норми ({max} °C) | above normal ({max} °C) | Червоне |
| `battery.ring.temp.sub.low` | нижче норми ({min} °C) | below normal ({min} °C) | Червоне |
| `battery.chart.title` | Ємність по місяцях, кВт·год | Capacity by month, kWh | |
| `battery.chart.note` | Ємність рахується з довгих розрядів — коли заряд падає на половину і більше. Перші місяці значення приблизні. | Capacity is estimated from long discharges (half the charge or more), so the first months are approximate. | Спека §7.5: ΔSOC ≥ 50 п.п. Макет казав «100% → 10%» — виправлено під спеку |
| `battery.chart.tooltip` | {month}: {capacity} кВт·год · розрядів у розрахунку: {count} | {month}: {capacity} kWh · based on {count, plural, one {# discharge} other {# discharges}} | |
| `battery.empty.title` | Ще замало даних | Not enough data yet | |
| `battery.empty.body` | Оцінка ємності з'явиться після першого довгого розряду — коли станція віддасть половину заряду або більше. | The capacity estimate appears after the first long discharge — half the charge or more. | |
| `battery.hidden.note` | Станція не передає {metric} — показник приховано. | The station doesn't report {metric}, so it's hidden. | Якщо показник недоступний з жодного джерела (§9 спеки) |

---

## 8. Енергія і джерела

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `energy.total.charged.label` | Заряджено | Charged | |
| `energy.total.discharged.label` | Віддано | Discharged | |
| `energy.total.value` | {kwh} кВт·год | {kwh} kWh | |
| `energy.total.efficiency.label` | ККД циклу | Round-trip efficiency | en 21 — у дві строки дозволено |
| `energy.total.efficiency.value` | {percent}% | {percent}% | |
| `energy.chart.title` | кВт·год по днях | kWh by day | |
| `energy.chart.title.weeks` | кВт·год по тижнях | kWh by week | |
| `energy.chart.legend.charge` | заряд | charge | |
| `energy.chart.legend.discharge` | розряд | discharge | |
| `energy.tooltip.charge` | Заряд: {kwh} кВт·год | Charged: {kwh} kWh | |
| `energy.tooltip.charge.split` | з мережі {grid} · сонце {pv} | from grid {grid} · solar {pv} | Лише якщо є PV |
| `energy.tooltip.discharge` | Розряд: {kwh} кВт·год | Discharged: {kwh} kWh | |
| `energy.tooltip.discharge.outage` | під час відключень {kwh} | during outages {kwh} | |
| `energy.tooltip.peak` | Пік розряду: {watts} Вт о {time} | Peak discharge: {watts} W at {time} | |
| `energy.ring.split.label` | Мережа / сонце | Grid / solar | Лише якщо є PV |
| `energy.ring.split.sub` | з мережі · {pv}% сонце | from grid · {pv}% solar | |
| `energy.empty` | За цей період станція не заряджалась і не віддавала енергію. | The station didn't charge or discharge in this period. | |

| Ключ | UK | EN |
|---|---|---|
| `source.lan` | LAN | LAN |
| `source.cloud` | хмара | cloud |
| `source.ble` | Bluetooth | Bluetooth |
| `source.none` | немає зв'язку | no connection |
| `age.now` | щойно | just now |
| `age.seconds` | {seconds} с тому | {seconds} s ago |
| `age.minutes` | {minutes} хв тому | {minutes} min ago |
| `age.hours` | {hours} год тому | {hours} h ago |
| `age.otherDay` | {date} о {time} | {date} at {time} |

Скорочені одиниці («с», «хв», «год») не змінюються за числом, тож `age.*` не потребують `plural`.

---

## 9. Формати: числа, тривалості, дати (ICU)

### 9.1 Числа

- Форматує `Intl.NumberFormat(locale)`. uk: десяткова **кома** («3,7 з 5,12 кВт·год»), en: крапка. Макети з крапкою — див. §14, п. 2.
- Заряд, %: ціле. Енергія: 1 знак після коми (3,7), ємність моделі — як у профілі (5,12). Потужність: ціле, групування розрядів у uk — нерозривний пробіл («2 500 Вт»), у en — кома («2,500 W»).
- Одиниці: uk **Вт, кВт·год, °C, %**; en **W, kWh, °C, %**. Між числом і одиницею — нерозривний пробіл (U+00A0), крім «%» і «°».

### 9.2 Тривалості

Три форми, обирає код за місцем:

| Форма | Де | UK | EN | Ключ |
|---|---|---|---|---|
| Годинникова | центр кілець-прогнозів, < 24 год | 7:30 | 7:30 | `overview.ring.eta.value.hm` |
| Коротка | підписи, таблиці, тултіпи, трей, сповіщення | 2 год 05 хв · 45 хв · 1 д 3 год | 2 h 05 min · 45 min · 1 d 3 h | `duration.short.*` |
| Довга | доступні назви (екранний диктор) | 2 години 5 хвилин | 2 hours 5 minutes | `duration.long.*` |

```json
"duration.short.m":   "{m} хв",
"duration.short.hm":  "{h} год {mm} хв",
"duration.short.h":   "{h} год",
"duration.short.dh":  "{d} д {h} год",

"duration.long.h":  "{h, plural, one {# година} few {# години} many {# годин} other {# години}}",
"duration.long.m":  "{m, plural, one {# хвилина} few {# хвилини} many {# хвилин} other {# хвилини}}",
"duration.long.d":  "{d, plural, one {# день} few {# дні} many {# днів} other {# дня}}",
"duration.long.hm": "{hours} {minutes}"
```

```json
"duration.short.m":   "{m} min",
"duration.short.hm":  "{h} h {mm} min",
"duration.short.h":   "{h} h",
"duration.short.dh":  "{d} d {h} h",

"duration.long.h":  "{h, plural, one {# hour} other {# hours}}",
"duration.long.m":  "{m, plural, one {# minute} other {# minutes}}",
"duration.long.d":  "{d, plural, one {# day} other {# days}}",
"duration.long.hm": "{hours} {minutes}"
```

Правила вибору (код, не рядок):
- < 1 хв → «< 1 хв» / «< 1 min» (`duration.short.lt1m`: `< 1 хв` / `< 1 min`).
- < 1 год → `m`; < 24 год → `hm` (хвилини з нулем: «2 год 05 хв»); рівно години → `h`; ≥ 24 год → `dh` («1 д 3 год»), хвилини відкидаються.
- **Прогнози в сповіщеннях і тексті** округлюються: < 3 год — до 5 хв («≈ на 1 год 50 хв»), ≥ 3 год — до години («≈ на 10 год»). Точність прогнозу не вища за це.
- `{mm}` — рядок із нулем попереду; `{h}`, `{d}` — числа. Тривалість ніколи не склеюється з окремих рядків у коді.

Українські відмінки в прикладах (`one/few/many/other`): 1 година · 2 години · 5 годин · 21 година · 1,5 години. 1 цикл · 3 цикли · 11 циклів. 1 раз · 2 рази · 5 разів. 1 нова · 2 нові · 5 нових.

### 9.3 Дати й час

24-годинний формат в обох мовах (див. §14, п. 6). Тиждень з понеділка.

| Ключ | UK | EN (en-GB) | Де |
|---|---|---|---|
| `date.time` | 14:05 | 14:05 | скрізь |
| `date.at` | о {time} | at {time} | «≈ о 22:45», «станція, ймовірно, розрядилась ≈ о 00:55» |
| `date.today` | сьогодні {time} | today {time} | таблиці |
| `date.yesterday` | вчора {time} | yesterday {time} | |
| `date.short` | 21.11 | 21 Nov | осі, таблиці |
| `date.shortTime` | 21.11 {time} | 21 Nov {time} | |
| `date.shortYear` | 21.11.2026 | 21 Nov 2026 | якщо не поточний рік |
| `date.weekdayShort` | пт | Fri | осі: «пт 15.11» / «Fri 15» |
| `date.long` | {weekdayLong}, {day} {monthGenitive} | {weekdayLong}, {day} {month} | тултіпи: «Пт, 15 листопада» / «Fri, 15 November» |
| `date.month` | листопад 2026 | November 2026 | графік ємності; осі — «лист», «Nov» |
| `date.range` | {start} – {end} | {start} – {end} | «15.11 – 21.11» / «15 – 21 Nov» |

- Місяці в uk — з малої літери, у датах — родовий відмінок («15 листопада»); дні тижня з малої, крім початку речення/тултіпа.
- Скорочення місяців uk для осі графіка ємності: січ, лют, бер, квіт, трав, черв, лип, серп, вер, жовт, лист, груд (з `Intl`, `month: 'short'`; макетне «верес» замінити на `Intl`-варіант).
- Усі часи — у часовому поясі Windows.

---

## 10. Кілька станцій і додавання станції

### 10.1 Перемикач і картки

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `stations.switcher.current` | {station} · {model} | {station} · {model} | Над вкладками, ▾ декоративна |
| `stations.card.title` | {station} | {station} | |
| `stations.card.model` | {model} | {model} | |
| `stations.card.pill.grid.on` | світло є | grid on | |
| `stations.card.pill.grid.off` | без світла {h}:{mm} | grid off {h}:{mm} | Пігулка тісна — годинникова форма (макет «без світла 1:12») |
| `stations.card.pill.power` | ↓ {watts} Вт / ↑ {watts} Вт / очікування | ↓ {watts} W / ↑ {watts} W / idle | ↓ розряд, ↑ заряд. a11y — `state.withPower.*` |
| `stations.card.pill.source` | {source} · {age} | {source} · {age} | |
| `stations.card.a11y` | {station}, заряд {soc}%, {gridState}, {powerState}, дані: {source}, {age} | {station}, charge {soc}%, {gridState}, {powerState}, data: {source}, {age} | |
| `stations.menu.add` | ＋ Додати станцію | ＋ Add station | |
| `stations.menu.manage` | Керувати станціями… | Manage stations… | |

### 10.2 Вікно «Додати станцію»

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `addStation.title` | Додати станцію | Add station | |
| `addStation.intro` | Одна станція може мати і локальне, і хмарне підключення — додаток сам бере найсвіжіші дані. | A station can have both a local and a cloud connection — the app uses whichever is freshest. | |
| `addStation.tile.lan.title` | Пошук у локальній мережі | Search local network | Макет: «Пошук у мережі» — уточнено за глосарієм |
| `addStation.tile.lan.body` | Знайде станції у вашому Wi-Fi. Найшвидші дані, працює без інтернету. | Finds stations on your Wi-Fi. Fastest data, works without internet. | |
| `addStation.tile.lan.badge` | рекомендовано | recommended | |
| `addStation.tile.cloud.title` | Акаунт Marstek | Marstek account | |
| `addStation.tile.cloud.body` | Увійдіть тим самим акаунтом, що й у телефоні. Працює звідусіль і дає історію. | Sign in with the same account as on your phone. Works from anywhere and provides history. | |
| `addStation.tile.ip.title` | IP вручну | Enter IP | |
| `addStation.tile.ip.body` | Якщо пошук не знайшов станцію. | If search didn't find the station. | |
| `addStation.tile.ble.title` | Bluetooth | Bluetooth | Плитка лише якщо є адаптер |
| `addStation.tile.ble.body` | Напряму, без Wi-Fi. Підтримка Venus E 3.0 не перевірена. | Direct, no Wi-Fi needed. Venus E 3.0 support isn't confirmed. | |
| `addStation.ble.unavailable` | На цьому комп'ютері немає Bluetooth-адаптера. | This PC has no Bluetooth adapter. | Пояснення неактивної опції (§7.2) |
| `addStation.scan.progress` | Шукаємо станції у вашому Wi-Fi… | Looking for stations on your Wi-Fi… | |
| `addStation.scan.found.title` | Знайдено в мережі: | Found on your network: | |
| `addStation.scan.found.row` | {model} · {ip} · {modelCode} · прошивка {firmware} | {model} · {ip} · {modelCode} · firmware {firmware} | «Venus E 3.0 · 192.168.1.47 · VNSE3-0 · прошивка v151» |
| `addStation.scan.found.untested` | може працювати, не тестувалося | may work, not tested | Модель без профілю «протестовано» (§7.6) |
| `addStation.scan.found.added` | вже додано | already added | Замість кнопки |
| `addStation.scan.found.action` | Додати | Add | |
| `addStation.scan.notListed` | Не бачите своєї? Локальний API треба ввімкнути на станції. | Don't see yours? The local API must be turned on in the station. | + `common.action.howTo` |
| `addStation.scan.none.title` | Станцій у локальній мережі не знайдено | No stations found on the local network | |
| `addStation.scan.none.body` | Найчастіше на станції не ввімкнено локальний API, або станція й цей комп'ютер у різних мережах Wi-Fi. | Usually the local API isn't turned on in the station, or the station and this PC are on different Wi-Fi networks. | |
| `addStation.scan.none.actions` | Шукати знову · Ввести IP · Увійти в акаунт Marstek | Search again · Enter IP · Sign in to Marstek | |
| `addStation.name.label` | Назва станції | Station name | |
| `addStation.name.help` | Так станція підписана в треї та сповіщеннях. | This name appears in the tray and notifications. | |
| `addStation.name.placeholder` | Дім | Home | Приклад, не значення за замовчуванням |
| `addStation.name.error.empty` | Дайте станції назву — так її буде видно в сповіщеннях. | Give the station a name — it's shown in notifications. | |
| `addStation.name.error.tooLong` | Назва — до {max} символів. Зараз {length}. | Up to {max} characters. This one is {length}. | max = 24 (бюджет трею) |
| `addStation.name.error.duplicate` | Станція з такою назвою вже є. | A station with this name already exists. | |
| `addStation.ip.label` | IP-адреса станції | Station IP address | |
| `addStation.ip.help` | Її видно в налаштуваннях роутера, у списку пристроїв. | You can find it in your router's list of devices. | |
| `addStation.ip.placeholder` | 192.168.1.47 | 192.168.1.47 | |
| `addStation.ip.error.format` | Введіть адресу у форматі 192.168.1.47. | Enter an address like 192.168.1.47. | |
| `addStation.ip.checking` | Перевіряємо зв'язок зі станцією… | Checking the connection… | |
| `addStation.ip.error.noAnswer` | За адресою {ip} станція не відповідає. Перевірте адресу і чи ввімкнено на станції локальний API. | No station answered at {ip}. Check the address and that the local API is turned on. | + `common.action.howTo` |
| `addStation.added.announce` | Станцію «{station}» додано. | Station “{station}” added. | Live region; у вікні видно картку — окремий тост не потрібен |

### 10.3 Підказка: як увімкнути локальний API

| Ключ | UK | EN |
|---|---|---|
| `help.localApi.title` | Як увімкнути локальний API | How to turn on the local API |
| `help.localApi.body` | Локальний API вмикається на самій станції — через застосунок Marstek Venus Monitor (по Bluetooth) або через підтримку Marstek. Покрокова інструкція — у README проєкту. | The local API is turned on in the station itself — with the Marstek Venus Monitor app (over Bluetooth) or through Marstek support. Step-by-step instructions are in the project README. |
| `help.localApi.sameWifi` | Станція і цей комп'ютер мають бути в одній мережі Wi-Fi. | The station and this PC must be on the same Wi-Fi network. |
| `help.localApi.firewall` | Якщо Windows запитає дозвіл для Marstek Monitor, дозвольте доступ у приватних мережах. | If Windows asks about Marstek Monitor, allow access on private networks. |
| `help.localApi.link` | Відкрити інструкцію | Open instructions |

Покрокові кроки свідомо винесено в README: інструмент і порядок дій залежать від прошивки й можуть змінитись; у додатку вони застаріли б непомітно.

### 10.4 Вхід у хмару Marstek

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `cloud.login.title` | Вхід в акаунт Marstek | Sign in to Marstek | |
| `cloud.login.email.label` | Електронна пошта | Email | |
| `cloud.login.password.label` | Пароль | Password | |
| `cloud.login.password.show.a11y` | Показати пароль | Show password | |
| `cloud.login.privacy` | Пароль зберігається в диспетчері облікових даних Windows і передається лише на сервери Marstek. | Your password is stored in Windows Credential Manager and sent only to Marstek's servers. | Факт зі спеки §8 — не міняти без перевірки реалізації |
| `cloud.login.unofficial` | Marstek не має офіційного доступу для сторонніх програм, тож вхід може перестати працювати після оновлень Marstek. | Marstek has no official API for third-party apps, so sign-in may stop working after Marstek updates. | Чесно про §3 спеки |
| `cloud.login.action` | Увійти | Sign in | |
| `cloud.login.progress` | Входимо… | Signing in… | |
| `cloud.login.success.stations` | {count, plural, one {Знайдено # станцію} few {Знайдено # станції} many {Знайдено # станцій} other {Знайдено # станції}} в акаунті. Виберіть, які додати. | {count, plural, one {Found # station} other {Found # stations}} in this account. Choose which to add. | |
| `cloud.login.success.none` | В акаунті немає станцій. | There are no stations in this account. | |
| `cloud.login.success.merge` | Ця станція вже додана локально — хмару підключено до неї. | This station is already added locally — the cloud connection was linked to it. | |

---

## 11. Налаштування

Розділи (§5.8): `settings.section.general` Загальні / General · `settings.section.notifications` Сповіщення / Notifications · `settings.section.stations` Станції / Stations · `settings.section.connection` Підключення / Connection · `settings.section.data` Дані та історія / Data and history · `settings.section.about` Про програму / About.

Підпис перемикача описує результат, коли ввімкнено; опис — наслідок. Ніяких заперечних формулювань.

**Іконки в Налаштуваннях — лінійні, без емодзі** (рішення власника 2026-11-21). Підписи `settings.notify.*` — лише текст («Зникло світло»). Лінійна іконка — окремий елемент дизайну, не частина рядка: один набір, нейтральний колір (hi-fi `07-settings.html`). Емодзі лишаються тільки в самих тостах Windows (§12.3).

### 11.1 Загальні

| Ключ | UK — підпис | UK — опис | EN — label | EN — description |
|---|---|---|---|---|
| `settings.general.language` | Мова | За замовчуванням — мова Windows. | Language | Defaults to your Windows language. |
| `settings.general.language.uk` / `.en` | Українська / English | | Українська / English | Назви мов — кожна своєю мовою |
| `settings.general.autostart` | Запускати з Windows | Щоб відключення записувались, щойно ввімкнеться комп'ютер. | Start with Windows | So outages are recorded as soon as the PC turns on. |
| `settings.general.minimizeToTray` | Згортати в трей при закритті | Вікно ховається, а додаток далі стежить за станцією. | Minimize to tray on close | The window hides; the app keeps watching the station. |
| `settings.general.theme` | Тема | Світла тема з'явиться пізніше. | Theme | A light theme is coming later. |
| `settings.general.theme.dark` | Темна | | Dark | |

### 11.2 Сповіщення

| Ключ | UK — підпис | UK — опис | EN — label | EN — description |
|---|---|---|---|---|
| `settings.notify.intro` | Спливають у кутку екрана, навіть коли вікно додатка закрите. | | Appear in the corner of the screen, even when the app window is closed. | |
| `settings.notify.test` | Надіслати тестове сповіщення | Перевірити, що Windows показує сповіщення (наприклад, що їх не приховує режим зосередження). | Send test notification | Check that Windows shows notifications (for example, that Focus isn't hiding them). |
| `settings.notify.test.sent` | Надіслано. Не з'явилось? Відкрити налаштування сповіщень Windows | | Sent. Didn't see it? Open Windows notification settings | |
| `settings.notify.group.grid` | Світло | | Grid | |
| `settings.notify.gridLost` | Зникло світло | З прогнозом, на скільки вистачить заряду. | Grid lost | With an estimate of how long the charge will last. |
| `settings.notify.gridBack` | Світло з'явилось | Скільки тривало відключення і коли станція зарядиться. | Grid back | How long the outage lasted and when the station will be full. |
| `settings.notify.group.battery` | Батарея | | Battery | |
| `settings.notify.low` | Низький заряд | Коли заряд опускається до порогу. Приходить і в тиху годину. | Low charge | When charge drops to the threshold. Comes through during quiet hours too. |
| `settings.notify.low.threshold` | Поріг сповіщення | 10% · 20% · 30% · 40% · 50% (за замовчуванням 20%) | Notification threshold | 10% · 20% · 30% · 40% · 50% (default 20%) |
| `settings.notify.full` | Станція заряджена до 100% | Один раз, коли заряд досягне 100%. | Station fully charged | Once, when charge reaches 100%. |
| `settings.notify.likelyOut` | Станція, ймовірно, розрядилась | Лише коли цей комп'ютер живиться не від станції і бачить це наживо. Увімкнено за замовчуванням. | Station probably ran out | Only when this PC isn't powered by the station and sees it happen live. On by default. |
| `settings.notify.error` | Нова помилка або попередження | З поясненням і порадою, що робити. | New error or warning | With an explanation and what to do. |
| `settings.notify.group.connection` | Зв'язок | | Connection | |
| `settings.notify.noConnection` | Немає зв'язку зі станцією | Якщо даних немає ні локально, ні з хмари довше, ніж вказано. Вимкнено за замовчуванням: без світла роутер теж може вимкнутись. | No connection to station | If there's no data from local or cloud for longer than set. Off by default: the router may lose power in an outage too. |
| `settings.notify.noConnection.after` | Після | 5 · 10 · 15 · 30 · 60 хв (за замовчуванням 10 хв) | After | 5 · 10 · 15 · 30 · 60 min (default 10 min) |
| `settings.notify.quiet` | Тиха година | Вночі сповіщення не спливають і не звучать, але їх видно в Центрі сповіщень. «Низький заряд» спливає завжди. | Quiet hours | At night, notifications don't pop up or make a sound, but they wait in the Notification Center. Low charge always pops up. |
| `settings.notify.quiet.range` | {start} – {end} | За замовчуванням 23:00 – 07:00 | {start} – {end} | Default 23:00 – 07:00 |
| `settings.notify.osDisabled` | Сповіщення для Marstek Monitor вимкнено в Windows. Відкрити налаштування Windows | | Notifications for Marstek Monitor are off in Windows. Open Windows settings | Банер, якщо ОС блокує |

### 11.3 Станції

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `settings.stations.intro` | Назви й порядок — як у перемикачі над вкладками. | Names and order as shown in the station switcher. | |
| `settings.stations.trayShows` | У треї показується: {station} | Tray shows: {station} | Вибрана станція (§5.7) |
| `settings.stations.rename.a11y` | Перейменувати {station} | Rename {station} | |
| `settings.stations.moveUp.a11y` / `.moveDown.a11y` | Перемістити {station} вище / нижче | Move {station} up / down | |
| `settings.stations.remove` | Видалити | Remove | Кнопка в рядку станції; відкриває діалог нижче |
| `settings.stations.empty` | Станцій ще немає. Додати станцію | No stations yet. Add station | |

**Діалог «Прибрати станцію»** — рішення власника (flows Q1): за замовчуванням архів, історія лишається; видалення історії — окрема явна дія з другим підтвердженням.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `settings.stations.remove.title` | Прибрати станцію «{station}»? | Remove “{station}”? | |
| `settings.stations.remove.body` | Додаток перестане стежити за нею. Сама станція й дані в хмарі Marstek не зміняться. | The app will stop monitoring it. The station itself and its data in the Marstek cloud won't change. | |
| `settings.stations.remove.option.archive` | Припинити стежити | Stop monitoring | Варіант за замовчуванням |
| `settings.stations.remove.option.archive.help` | Історія лишається в «Станції → Архів»; станцію можна повернути. | History stays in Stations → Archive, and you can bring the station back. | |
| `settings.stations.remove.option.delete` | Видалити разом з історією | Remove with history | |
| `settings.stations.remove.option.delete.help` | Історія за {days, plural, one {# день} few {# дні} many {# днів} other {# дня}}: {outages, plural, one {# відключення} few {# відключення} many {# відключень} other {# відключення}}, журнал помилок і заряду. Скасувати це не можна. | History for {days, plural, one {# day} other {# days}}: {outages, plural, one {# outage} other {# outages}}, plus the error and charge logs. This can't be undone. | Числа — реальні, з бази |
| `settings.stations.remove.action.archive` | Припинити стежити | Stop monitoring | Основна кнопка праворуч |
| `settings.stations.remove.action.delete` | Видалити з історією… | Remove with history… | Червоний контур ліворуч; «…» = далі ще одне підтвердження |
| `settings.stations.remove.action.cancel` | Скасувати | Cancel | |
| `settings.stations.remove.undo.text` | «{station}» перенесено в архів. | “{station}” moved to the archive. | Нижній рядок, 10 с, таймер стоїть під курсором/фокусом |
| `settings.stations.remove.undo.action` | Повернути | Undo | Макет мав «Скасувати» — замінено: у тому ж потоці «Скасувати» вже означає «закрити діалог» |
| `settings.stations.remove.undo.a11y` | «{station}» перенесено в архів. Повернути можна протягом 10 секунд. | “{station}” moved to the archive. You can undo this for 10 seconds. | Live region, polite |
| `settings.stations.deleteHistory.title` | Видалити станцію «{station}» разом з історією? | Remove “{station}” and its history? | Друге підтвердження |
| `settings.stations.deleteHistory.body` | З цього комп'ютера буде стерто {outages, plural, one {# відключення} few {# відключення} many {# відключень} other {# відключення}} за {days, plural, one {# день} few {# дні} many {# днів} other {# дня}}, журнал помилок і заряду. Станція й дані в хмарі Marstek не зміняться. Скасувати це не можна. | This erases {outages, plural, one {# outage} other {# outages}} over {days, plural, one {# day} other {# days}}, plus the error and charge logs, from this PC. The station and its Marstek cloud data won't change. This can't be undone. | |
| `settings.stations.deleteHistory.export` | Спочатку експортувати в CSV | Export to CSV first | Нейтральна дія |
| `settings.stations.deleteHistory.keep` | Залишити історію | Keep history | Безпечна, не первинний стиль |
| `settings.stations.deleteHistory.action` | Видалити назавжди | Delete permanently | Деструктивний стиль; без undo |
| `settings.stations.deleteHistory.done` | Історію станції «{station}» видалено. | “{station}” history deleted. | Live region; без undo — дія вже підтверджена двічі |

**Архів**

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `settings.stations.archive.title` | Архів | Archive | |
| `settings.stations.archive.intro` | За цими станціями додаток не стежить, але їхня історія збережена. | The app doesn't monitor these stations, but their history is kept. | |
| `settings.stations.archive.row` | {station} · {model} · стежили до {date} | {station} · {model} · monitored until {date} | |
| `settings.stations.archive.restore` | Повернути | Restore | Станція знову в перемикачі, історія продовжується, пропуск — з причиною «не стежили» |
| `settings.stations.archive.deleteHistory` | Видалити історію… | Delete history… | Відкриває `settings.stations.deleteHistory.*` |
| `settings.stations.archive.empty` | В архіві порожньо. | The archive is empty. | |
| `gap.label.archived` | не стежили | not monitored | Причина пропуску на графіках після повернення з архіву |

### 11.4 Підключення

| Ключ | UK — підпис | UK — опис / значення | EN — label | EN — description / value |
|---|---|---|---|---|
| `settings.conn.cloud.title` | Хмара Marstek | | Marstek cloud | |
| `settings.conn.cloud.signedIn` | Увійшли як {email} | Вийти | Signed in as {email} | Sign out |
| `settings.conn.cloud.signedOut` | Не підключено | Увійти — для історії, поки комп'ютер вимкнений, і доступу не з дому. | Not connected | Sign in — for history while this PC is off, and access away from home. |
| `settings.conn.cloud.signOut.confirm` | Вийти з акаунта Marstek? | Дані будуть оновлюватись лише локально. Вже збережена історія залишиться. [Залишитись] [Вийти] | Sign out of Marstek? | Data will update only over the local network. Saved history stays. [Stay signed in] [Sign out] |
| `settings.conn.poll` | Опитування локально | кожні 2 с · 5 с · 10 с · 30 с (за замовчуванням 5 с). Рідше — менше навантаження на станцію. | Local polling | every 2 s · 5 s · 10 s · 30 s (default 5 s). Less often means less load on the station. |
| `settings.conn.ble` | Bluetooth | {available, select, true {Адаптер є} other {На цьому комп'ютері немає Bluetooth-адаптера}} | Bluetooth | {available, select, true {Adapter found} other {This PC has no Bluetooth adapter}} |
| `settings.conn.station.row` | {station}: {sources} | «Дім: LAN · хмара» | {station}: {sources} | |

### 11.5 Дані та історія

| Ключ | UK — підпис | UK — опис / значення | EN — label | EN — description / value |
|---|---|---|---|---|
| `settings.data.retention` | Зберігати історію | завжди · 1 рік · 2 роки · 5 років (за замовчуванням — завжди) | Keep history | forever · 1 year · 2 years · 5 years (default forever) |
| `settings.data.retention.confirm` | Видалити історію, старшу за {period}? | Записи до {date} буде стерто з цього комп'ютера. Скасувати це не можна. [Залишити] [Видалити старіші записи] | Delete history older than {period}? | Records before {date} will be erased from this PC. This can't be undone. [Keep] [Delete older records] |
| `settings.data.export` | Експортувати в CSV | Відключення, помилки й заряд за вибраний період — для Excel чи Google Таблиць. | Export to CSV | Outages, errors and charge for a chosen period — for Excel or Google Sheets. |
| `settings.data.export.done` | Збережено: {fileName} · Показати в папці | | Saved: {fileName} · Show in folder | |
| `settings.data.location` | Дані зберігаються тільки на цьому комп'ютері: {path} | | Data is stored only on this PC: {path} | |

### 11.6 Про програму

| Ключ | UK | EN |
|---|---|---|
| `settings.about.version` | Версія {version} | Version {version} |
| `settings.about.checkUpdates` | Перевірити оновлення | Check for updates |
| `settings.about.autoUpdates` | Перевіряти оновлення автоматично — Раз на день запит до GitHub, без ваших даних. | Check for updates automatically — Once a day, a request to GitHub with none of your data. |
| `settings.about.update.checking` | Перевіряємо… | Checking… |
| `settings.about.update.latest` | У вас остання версія. | You have the latest version. |
| `settings.about.update.available` | Доступна версія {version}. Завантажити з GitHub | Version {version} is available. Download from GitHub |
| `settings.about.update.failed` | Не вдалося перевірити — немає зв'язку з GitHub. Спробуйте пізніше. | Couldn't check — GitHub isn't reachable. Try again later. |
| `settings.about.update.checkNow` | Перевірити зараз | Check now |
| `settings.about.update.install` | Оновити | Update |
| `settings.about.update.failed.v1` | Не вдалося перевірити оновлення. Спробуйте пізніше. | Couldn't check for updates. Try again later. |
| `settings.about.update.simulated` | Перевірка оновлень вимкнена в імітації. | Update checks are off in a simulated run. |
| `settings.about.update.dev` | Збірка розробника — оновлення не перевіряються. | Developer build — updates aren't checked. |
| `settings.about.privacy` | Додаток не надсилає ваші дані нікуди, крім станції та серверів Marstek. Телеметрії немає. | The app sends your data nowhere except to the station and Marstek's servers. No telemetry. |
| `settings.about.description` | Моніторинг домашньої зарядної станції Marstek: заряд, відключення електромережі, помилки. Неофіційний додаток, не пов'язаний з Marstek. | Monitoring for Marstek home battery stations: charge, grid outages, errors. Unofficial, not affiliated with Marstek. |
| `settings.about.github` | Код на GitHub | Source on GitHub |

---

## 12. Трей і сповіщення

### 12.1 Тултіп іконки

Ліміт Windows — **127 символів** (`szTip` = 128 з нульовим символом). Назва станції в тултіпі обрізається до 24 символів (`addStation.name` має такий самий ліміт). Рядки — через `\n`.

| Ключ | UK | EN |
|---|---|---|
| `tray.tooltip.line1` | {station} · {model} | {station} · {model} |
| `tray.tooltip.line2` | {soc}% · {stateWithPower} | {soc}% · {stateWithPower} |
| `tray.tooltip.line3.off.toEmpty` | Без світла {duration} · вистачить на {eta} | Grid off {duration} · ~{eta} left |
| `tray.tooltip.line3.off.noEta` | Без світла {duration} | Grid off {duration} |
| `tray.tooltip.line3.on.charging` | Світло є · до 100% о {time} | Grid on · full at {time} |
| `tray.tooltip.line3.on.idle` | Світло є | Grid on |
| `tray.tooltip.stale` | {station} · {model}\nДані застарілі, останні о {time}\n{lastSoc}% на той момент | {station} · {model}\nOut of date, last update at {time}\n{lastSoc}% at that time |
| `tray.tooltip.noConnection` | {station} · {model}\nНемає зв'язку зі станцією\nОстанні дані о {time} | {station} · {model}\nNo connection to station\nLast data at {time} |
| `tray.tooltip.noStations` | Marstek Monitor\nСтанцію ще не додано | Marstek Monitor\nNo station added yet |

`{eta}` і `{duration}` у треї — **коротка** форма («7 год 30 хв»), не годинникова: «вистачить на 7:30» читається як «до 7:30» (див. §14, п. 4).

Найгірший випадок uk: назва на 24 символи + « · Venus E 3.0» (24 + 15 = 39) + «72% · розряд 2 500 Вт» (21) + «Без світла 23 год 59 хв · вистачить на 23 год 55 хв» (51) + 2 переводи рядка = **113 ≤ 127**. en: 39 + 23 + 36 + 2 = 100.

### 12.2 Меню трею

Порядок і поведінка — flows §5.1. Меню нативне Win32: без емодзі, підменю позначає ОС (▸).

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `tray.menu.header` | {station} · {soc}% · {gridShort} | {station} · {soc}% · {gridShort} | Неактивний рядок-заголовок. «Дім · 72% · без світла 3:27» |
| `tray.menu.header.gridOn` | світло є | grid on | `{gridShort}`, коли світло є |
| `tray.menu.header.gridOff` | без світла {h}:{mm} | grid off {h}:{mm} | Годинникова форма — як у пігулці станції (тісне місце; «без світла» перед числом не дає прочитати його як час доби) |
| `tray.menu.header.stale` | дані о {time} | data from {time} | STALE / NO_LINK — без стану світла |
| `tray.menu.header.noStations` | Станцію ще не додано | No station added yet | |
| `tray.menu.open` | Відкрити Marstek Monitor | Open Marstek Monitor | Дія за замовчуванням (жирна) |
| `tray.menu.trayStation` | Станція в треї | Station in tray | Підменю, лише якщо станцій ≥ 2; пункти — `{station}`, вибрана з позначкою ОС |
| `tray.menu.trayStation.item` | {station} | {station} | |
| `tray.menu.dnd` | Не турбувати | Do not disturb | Підменю |
| `tray.menu.dnd.1h` | 1 год | 1 hour | |
| `tray.menu.dnd.3h` | 3 год | 3 hours | |
| `tray.menu.dnd.8h` | 8 год | 8 hours | |
| `tray.menu.dnd.untilMorning` | До ранку ({time}) | Until morning ({time}) | `{time}` — кінець тихої години або 07:00. Макет показує час окремою колонкою — тоді рядок без дужок: `До ранку` / `Until morning` |
| `tray.menu.dnd.untilOff` | Доки не ввімкну | Until I turn it back on | |
| `tray.menu.dnd.active` | Не турбувати до {time} · Вимкнути | Do not disturb until {time} · Turn off | Коли активно; один клік вимикає. Позначка ✓ — від ОС |
| `tray.menu.dnd.active.indefinite` | Не турбувати увімкнено · Вимкнути | Do not disturb is on · Turn off | Для «Доки не ввімкну» |
| `tray.menu.dnd.help` | «Низький заряд» приходить завжди. | Low charge always comes through. | Не в меню (Win32 не має підказок) — в описі в Налаштуваннях і в README |
| `tray.menu.update` | Доступне оновлення {version} | Update available: {version} | Лише якщо є (flows §6) |
| `tray.menu.settings` | Налаштування | Settings | |
| `tray.menu.quit` | Вийти | Quit | Відокремлений, внизу; без підтвердження (flows §5.2) |
| `tray.firstMinimize.toast.title` | Marstek Monitor працює в треї | Marstek Monitor is running in the tray | Лише перше закриття вікна |
| `tray.firstMinimize.toast.body` | І далі стежить за станцією. Щоб вийти, клацніть іконку правою кнопкою → «Вийти». | It keeps watching the station. To quit, right-click the icon → Quit. | |
| `settings.general.minimizeToTray.off.help` | Поки додаток закритий, відключення не записуються. | While the app is closed, outages aren't recorded. | Опис, коли перемикач вимкнено (flows §5.2) — замість діалогу при виході |
| `tray.icon.a11y` | {station}: {soc}%, {gridState} | {station}: {soc}%, {gridState} | |

Діалог підтвердження виходу (`tray.quit.confirm.*`) з попередньої версії **видалено**: flows §5.2 вирішує «вихід без підтвердження», а попередження дає опис перемикача й картка пропуску на наступному старті.

Тултіп при кількох станціях (flows §5.1): після трьох рядків вибраної станції — `tray.tooltip.more`: `+{count} ще` / `+{count} more`, якщо інакше перевищимо 127 символів.

### 12.3 Сповіщення Windows

**Назва станції.** Верхній рядок тоста Windows — це зареєстрована назва додатка («Marstek Monitor»), дописати до нього «· Дім» не можна. Тому назва станції йде в **нижній рядок атрибуції** (`<text placement="attribution">`), дрібним шрифтом під тілом:

```
🕯️  Marstek Monitor              ← ОС, не наш рядок
    Зникло світло                ← title
    15:17 · заряд 95% · …        ← body
    Дім · Venus E 3.0            ← attribution
```

Бюджети: заголовок ≤ 40 символів, тіло ≤ 90 (2 рядки), атрибуція ≤ 40. Емодзі — окреме поле (`icon`), не частина заголовка. Якщо реалізація не підтримує окрему іконку, емодзі стає префіксом заголовка через пробіл.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `notify.attribution` | {station} · {model} | {station} · {model} | Нижній рядок, у кожному тості. «Дім · Venus E 3.0» |
| `notify.attribution.short` | {station} | {station} | Якщо назва + модель > 40 |
| `notify.titleWithStation` | {station}: {titleLower} | {station}: {titleLower} | **Запасний варіант**, лише якщо атрибуція недоступна. «Дім: зникло світло». `titleLower` — заголовок з малої першої літери (код знижує лише перший символ). Бюджет заголовка тоді 40 − (назва + 2) |

| Ключ | Іконка | UK — заголовок | UK — тіло | EN — title | EN — body |
|---|---|---|---|---|---|
| `notify.gridLost` | 🕯️ | Зникло світло | {time} · заряд {soc}% · вистачить ≈ на {eta} | Grid lost | {time} · charge {soc}% · lasts ≈ {eta} |
| `notify.gridLost.noEta` | 🕯️ | Зникло світло | {time} · заряд {soc}% | Grid lost | {time} · charge {soc}% |
| `notify.gridLost.approx` | 🕯️ | Світла немає | Зникло між {from} і {to}, поки додаток не працював. Заряд {soc}% · вистачить ≈ на {eta} | Grid off | Went off between {from} and {to} while the app wasn't running. Charge {soc}% · lasts ≈ {eta} |
| `notify.gridLost.approx.pcSleep` | 🕯️ | Світла немає | Зникло між {from} і {to}, поки ПК спав. Заряд {soc}% · вистачить ≈ на {eta} | Grid off | Went off between {from} and {to} while the PC was asleep. Charge {soc}% · lasts ≈ {eta} |
| `notify.gridLost.approx.noLink` | 🕯️ | Світла немає | Зникло між {from} і {to}, поки не було зв'язку зі станцією. Заряд {soc}% · вистачить ≈ на {eta} | Grid off | Went off between {from} and {to} while there was no connection to the station. Charge {soc}% · lasts ≈ {eta} |
| `notify.gridLost.approx.noFrom` | 🕯️ | Світла немає | Заряд {soc}% · вистачить ≈ на {eta} | Grid off | Charge {soc}% · lasts ≈ {eta} |
| `notify.gridBack` | 💡 | Світло з'явилось | Без світла було {duration} · заряд {soc}% · до 100% ≈ о {time} | Grid back | Off for {duration} · charge {soc}% · full ≈ {time} |
| `notify.gridBack.full` | 💡 | Світло з'явилось | Без світла було {duration} · заряд 100% | Grid back | Off for {duration} · charge 100% |
| `notify.gridBack.stationWasOut` | 💡 | Світло з'явилось | Без світла було ≈ {duration}. Станція, ймовірно, розрядилась о {diedAt} · зараз {soc}% | Grid back | Off for ≈ {duration}. Station probably ran out at {diedAt} · now {soc}% |
| `notify.low` | 🪫 | Низький заряд — {soc}% | При {watts} Вт вистачить ≈ на {eta} | Low charge — {soc}% | At {watts} W, lasts ≈ {eta} |
| `notify.low.gridOn` | 🪫 | Низький заряд — {soc}% | Світло є, станція заряджається. | Low charge — {soc}% | Grid is on, the station is charging. |
| `notify.full` | 🔋 | Станція заряджена | 100% · {capacity} кВт·год · вистачить ≈ на {eta} | Station fully charged | 100% · {capacity} kWh · lasts ≈ {eta} |
| `notify.full.noEta` | 🔋 | Станція заряджена | 100% · {capacity} кВт·год | Station fully charged | 100% · {capacity} kWh |
| `notify.test` | 🔔 | Тестове сповіщення | Якщо ви це бачите — сповіщення працюють. | Test notification | If you can see this, notifications are working. |

`notify.test` надсилається лише вручну, кнопкою `settings.notify.test`. Атрибуція — як у всіх тостах («Дім · Venus E 3.0»), а без станцій — `Marstek Monitor`. Тиху годину й «Не турбувати» тестове сповіщення ігнорує, бо людина саме перевіряє, чи сповіщення спливають. Режим зосередження Windows ми не обходимо: якщо Windows приховала тост, це і є відповідь на перевірку. Тому `settings.notify.test.sent` пропонує відкрити налаштування Windows. Клік по тесту відкриває Налаштування → Сповіщення.

| Ключ | Іконка | UK — заголовок | UK — тіло | EN — title | EN — body |
|---|---|---|---|---|---|
| `notify.likelyOut` | 🪫 | Станція, ймовірно, розрядилась | {time} · заряд перед цим {soc}% | Station probably ran out | {time} · charge before that {soc}% |
| `notify.gridFlapping` | 🕯️ | Світло нестабільне | За 10 хв зникало {count, plural, one {# раз} few {# рази} many {# разів} other {# рази}}. Наступне сповіщення — коли стабілізується. | Grid is unstable | Went off {count, plural, one {# time} other {# times}} in 10 min. Next notification once it settles. |
| `notify.error` | ⚠️ | Помилка: {title} ({code}) | {todo} | Error: {title} ({code}) | {todo} |
| `notify.warning` | ⚠️ | Попередження: {title} | {todo} | Warning: {title} | {todo} |
| `notify.error.unknown` | ⚠️ | Невідома помилка (код {code}) | Подробиці — на вкладці «Помилки». | Unknown error (code {code}) | Details on the Errors tab. |
| `notify.noConnection` | 📡 | Немає зв'язку зі станцією | Ні локально, ні через хмару вже {duration}. Останні дані о {time}. | No connection to station | Neither local nor cloud for {duration}. Last data at {time}. |
| `notify.cloudSession` | ⚠️ | Потрібно знову увійти в Marstek | Без цього історія, поки ПК вимкнений, не дозавантажиться. | Sign in to Marstek again | Otherwise history from while this PC was off won't be filled in. |

Правила:
- `{eta}` у сповіщеннях — округлена коротка форма (§9.2): «≈ на 10 год», «≈ на 1 год 50 хв».
- Клік по сповіщенню відкриває відповідну вкладку (gridLost/gridBack → Огляд; error → Помилки).
- Сповіщення не показують застарілі дані як поточні: якщо дані застарілі, `notify.gridLost` не надсилається з прогнозом — лише `noEta`.
- Тиха година і «Не турбувати» приглушують усе, крім `notify.low`: такі сповіщення не спливають, а йдуть у Центр сповіщень (flows D7).
- Сповіщення надсилаються лише з живих даних (flows D5). Події, відновлені заднім числом, тоста не дають — для них є картка `gapCard.*` (§13.5). Тому `notify.gridLost.approx` надсилається лише тоді, коли відключення ще триває на момент старту. Тост «Зникло світло» про подію, що була давно, не надсилається.
- `notify.gridLost.approx*` (власник, 2026-11-23): відключення знайдено після пропуску, тож його початок невідомий. Заголовок без часу; `{from}` — останній вимір перед пропуском (його початок), `{to}` — перше спостереження відключення після нього: світло зникло між ними. Час — «HH:MM»; якщо `{from}` в іншу місцеву добу — короткий формат дати й часу (§9.3: «вчора 23:40», «21.11 23:40»). `.pcSleep` — пропуск «ПК спав», `.noLink` — «немає зв'язку» або «ймовірно, розрядилась». `.noFrom` — до пропуску не було жодного виміру (найперший запуск): вікна назвати не можна. Без прогнозу частина « · вистачить ≈ на {eta}» випадає.
- `notify.likelyOut` надсилається **лише якщо виявлено наживо**: ПК живиться не від станції й бачить, як станція з низьким зарядом зникає з усіх джерел (flows Q4). Перемикач: `settings.notify.likelyOut`, увімкнений за замовчуванням. Якщо це виявлено заднім числом, тост не надсилається — лише картка пропуску, рядок у Відключеннях і запис у Помилках. `{time}` — час останнього виміру. «Ймовірно» прибирати не можна: це висновок, не факт.
- `notify.noConnection` і «Зв'язок відновлено»: при відновленні це не новий тост — той самий тег оновлює попередній на `notify.noConnection.restored` (нижче).

| Ключ | Іконка | UK — заголовок | UK — тіло | EN — title | EN — body |
|---|---|---|---|---|---|
| `notify.noConnection.restored` | 📡 | Зв'язок зі станцією відновлено | Не було {duration}. Заряд {soc}%. | Connection restored | Lost for {duration}. Charge {soc}%. |

**Свідомо не написано:** окремий тост «Зв'язок відновлено». Замість нього оновлюється попередній тост (flows §2.7), тож повідомлення про відновлення не з'являється як нове. Тост про оновлення додатка: був «ніколи не toast» (flows §6.1); план 3 за рішенням власника додає один тост на нову версію (`notify.update`, §13.6).

---

## 13. Онбординг, порожні стани, офлайн

Ключі для `…-flows.md` (UX flow designer). Які екрани існують і в якому порядку — рішення flow; тут — слова.

### 13.1 Перший запуск

| Ключ | UK | EN |
|---|---|---|
| `onboarding.welcome.title` | Marstek Monitor | Marstek Monitor |
| `onboarding.welcome.body` | Показує, що зараз із зарядною станцією, на скільки її вистачить і як часто не було світла. Історія зберігається на цьому комп'ютері. | Shows how your station is doing, how long it will last and how often the grid was off. History is kept on this PC. |
| `onboarding.welcome.action` | Додати станцію | Add station |
| `onboarding.step` | Крок {current} з {total} | Step {current} of {total} |
| `onboarding.cloudOffer.title` | Підключити ще й хмару Marstek? | Also connect the Marstek cloud? |
| `onboarding.cloudOffer.body` | Тоді відключення, які були, поки комп'ютер вимкнений, теж потраплять в історію. | Then outages that happen while this PC is off will be in your history too. |
| `onboarding.cloudOffer.action` | Увійти в акаунт Marstek | Sign in to Marstek |
| `onboarding.cloudOffer.skip` | Пізніше — це можна зробити в Налаштуваннях | Later — you can do this in Settings |
| `onboarding.notifyOffer.title` | Сповіщення про світло | Grid notifications |
| `onboarding.notifyOffer.body` | Додаток скаже, коли зникне або з'явиться світло і коли заряд стане низьким. | The app will tell you when the grid goes off or comes back, and when charge gets low. |
| `onboarding.done.action` | Перейти до огляду | Go to overview |
| `onboarding.firstData.waiting` | Чекаємо перших даних від станції… | Waiting for the first data from the station… |

Перший запуск — це одна сторінка, а не майстер (flows §1.3). Тому `onboarding.welcome.title`/`.action` і `onboarding.step` використовуються лише тоді, коли flow колись повернеться до кроків. Зараз вгорі — один рядок `onboarding.welcome.body`, а під ним — плитки `addStation.tile.*`.

**NONE_FOUND і діагноз мережі** (flows §1.5)

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `onboarding.none.title` | = `addStation.scan.none.title` | | |
| `onboarding.none.body` | = `addStation.scan.none.body` | | Найімовірніша причина — першою |
| `onboarding.none.action.cloud` | Увійти в акаунт Marstek | Sign in to Marstek | **Головна** дія |
| `onboarding.none.action.howTo` | Як увімкнути локальний API | How to turn on the local API | Кнопка (друга дія), веде на `LAN_GUIDE` |
| `onboarding.none.action.ip` | Ввести IP | Enter IP | Третя дія |
| `onboarding.none.action.rescan` | Шукати знову | Search again | Текстове посилання |
| `onboarding.dx.netPublic.title` | Мережа Windows позначена як «Загальнодоступна» | Windows marks this network as Public | Блок діагнозу замість загальної поради |
| `onboarding.dx.netPublic.body` | У такій мережі Windows блокує відповіді станції. Змініть профіль мережі на «Приватна». | On a public network, Windows blocks the station's replies. Change the network profile to Private. | + `common.action.howTo` (відкриває налаштування мережі Windows) |
| `onboarding.dx.firewall.title` | Брандмауер Windows може блокувати пошук | Windows Firewall may be blocking the search | |
| `onboarding.dx.firewall.body` | Дозвольте Marstek Monitor доступ у приватних мережах — у налаштуваннях брандмауера Windows. | Allow Marstek Monitor on private networks in Windows Firewall settings. | |
| `onboarding.dx.noLan.title` | Цей комп'ютер не в локальній мережі | This PC isn't on a local network | |
| `onboarding.dx.noLan.body` | Підключіть його до того самого Wi-Fi, що й станцію, або увійдіть в акаунт Marstek. | Connect it to the same Wi-Fi as the station, or sign in to Marstek. | |
| `onboarding.dx.otherSubnet.body` | Станція може бути в іншій підмережі (VPN, другий роутер). Спробуйте ввести її IP вручну. | The station may be on another subnet (VPN, a second router). Try entering its IP. | |

**LAN_GUIDE** — у тому самому вікні; основний текст — `help.localApi.*` (§10.3)

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `onboarding.lanGuide.after` | Після ввімкнення натисніть «Шукати знову» — тут же, нікуди не переходячи. | Once it's on, press Search again — right here. | |
| `onboarding.lanGuide.cloudFirst.title` | Поки що — почати з хмари | For now — start with the cloud | |
| `onboarding.lanGuide.cloudFirst.body` | Локальне підключення можна додати пізніше, до тієї самої станції. | You can add the local connection later, to the same station. | + `onboarding.none.action.cloud` |
| `onboarding.lanGuide.supportTemplate.action` | Скопіювати текст листа | Copy email text | Шлях «через підтримку Marstek» (flows §1.5) |
| `onboarding.lanGuide.supportTemplate.body` | Добрий день! Прошу увімкнути локальний API (Open API) на моїй станції {model}, серійний номер {serial}. Дякую. | Hello, please enable the local API (Open API) on my {model} station, serial number {serial}. Thank you. | Шаблон; людина вставляє в лист сама. `serial` — якщо відомий з хмари, інакше `[серійний номер]` |

Покрокову інструкцію для Marstek Venus Monitor (Web Bluetooth, обмеження iPhone/Safari) я свідомо не переношу в додаток: розробник має перевірити ці факти (flows §1.5), а потім вони йдуть у README. Кнопка `help.localApi.link` веде туди.

**CLOUD_INCOMPATIBLE / вхід**

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `onboarding.cloud.incompatible` | = `error.cloud.login.changed` | | Червона плашка під формою, + `error.reference` |
| `onboarding.cloud.incompatible.extra` | Хмара Marstek могла попросити додаткову перевірку (код, капчу) — додаток поки не вміє її проходити. | The Marstek cloud may have asked for extra verification (a code or captcha), which the app can't handle yet. | Коли відповідь схожа на 2FA/капчу |
| `onboarding.cloud.incompatible.lan` | Пошук у локальній мережі | Search local network | Друга дія |
| `onboarding.cloud.reportBug` | Повідомити розробника | Report to the developer | Відкриває GitHub issue з шаблоном |
| `onboarding.cloud.reportBug.help` | Відкриється сторінка на GitHub з версією додатка й кодом помилки. Пароль і пошта туди не потрапляють. | Opens a GitHub page with the app version and error code. Your password and email aren't included. | Факт — перевірити в реалізації шаблону |
| `onboarding.cloud.rateLimited` | Забагато спроб. Спробуйте знову через {duration}. | Too many attempts. Try again in {duration}. | Кнопка неактивна з відліком |
| `onboarding.cloud.resetPassword` | Забули пароль? Скинути його можна в застосунку Marstek на телефоні. | Forgot your password? Reset it in the Marstek phone app. | Після 3-ї невдалої спроби (flows O7) |
| `onboarding.cloud.empty` | = `cloud.login.success.none` + «Пошук у локальній мережі · Вийти з акаунта» | | `CLOUD_EMPTY` |

**DONE — станцію додано**

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `onboarding.done.title` | Станцію «{station}» додано | Station “{station}” added | Заголовок |
| `onboarding.done.summary` | {model} · {address} · {sources}. Перші дані: {soc}%, {gridState}. | {model} · {address} · {sources}. First data: {soc}%, {gridState}. | «Venus E 3.0 · 192.168.1.47 · LAN. Перші дані: 72%, без світла.» Для хмари `{address}` пропускається |
| `onboarding.done.summary.waiting` | {model} · {sources}. Чекаємо перших даних — це може зайняти до хвилини. | {model} · {sources}. Waiting for the first data — this can take up to a minute. | `ADDING` без даних за 20 с (flows O9) |
| `onboarding.done.offerCloud.title` | = `onboarding.cloudOffer.title` | | Якщо додано лише LAN |
| `onboarding.done.offerCloud.body` | Коли локальний API замовкає, дані йтимуть через хмару. А відключення, поки комп'ютер вимкнений, теж потраплять в історію. | When the local API goes quiet, data comes through the cloud. Outages while this PC is off get into your history too. | Розширює `onboarding.cloudOffer.body` |
| `onboarding.done.offerCloud.later` | Пізніше | Later | Закриває картку назавжди |
| `onboarding.done.offerLan.title` | Увімкнути ще й локальний доступ? | Also turn on local access? | Якщо додано лише хмару |
| `onboarding.done.offerLan.body` | Дані кожні кілька секунд і без інтернету. Для цього на станції треба ввімкнути локальний API. | Data every few seconds, no internet needed. This requires turning on the local API on the station. | Flows казав «щосекунди» — замінено: опитування за замовчуванням 5 с |
| `onboarding.done.trayHint` | Windows 11 ховає нові іконки під «^». Перетягніть батарейку на панель задач, щоб бачити заряд завжди. | Windows 11 hides new icons under “^”. Drag the battery onto the taskbar to always see the charge. | Необов'язкова, закривається |
| `onboarding.done.trayHint.dismiss.a11y` | Закрити підказку | Dismiss tip | |
| `onboarding.done.addAnother` | Додати ще одну станцію | Add another station | |
| `onboarding.done.name.label` | = `addStation.name.label` / `.help` | | Редагується на місці, ≤ 24 |
| `onboarding.done.autostart` / `.minimizeToTray` | = `settings.general.autostart` / `.minimizeToTray` | | Перемикачі на місці |
| `onboarding.languageSwitch.a11y` | Мова інтерфейсу | Interface language | Маленький перемикач у куті |
| `addStation.scan.found.added.open` | Відкрити | Open | Замість «Додати» для вже доданої (flows O1) |
| `addStation.scan.found.serial` | …{serialTail} | …{serialTail} | Останні 4 символи серійника, коли знайдено кілька (flows O2) |
| `addStation.scan.found.merged` | LAN + хмара | LAN + cloud | Одна станція з обох джерел (flows O3) |
| `addStation.ip.error.notMarstek` | За адресою {ip} відповів пристрій, але це не станція Marstek. | A device answered at {ip}, but it isn't a Marstek station. | `PROBING_IP` → `non_marstek_reply` |

### 13.2 Порожні стани (зведення)

| Ключ | UK | EN |
|---|---|---|
| `empty.noStations.title` | Станцію ще не додано | No station added yet |
| `empty.noStations.body` | Додайте станцію — через Wi-Fi, акаунт Marstek або за IP. | Add a station — via Wi-Fi, your Marstek account or by IP. |
| `empty.noStations.action` | Додати станцію | Add station |
| `empty.forecast.noHistory` | ще немає даних | not enough data yet |

Інші порожні стани — у своїх розділах: `outages.empty.*`, `errors.empty.*`, `battery.empty.*`, `energy.empty`, `overview.chart.empty`.

### 13.3 Офлайн і зв'язок

| Ключ | UK | EN | Де |
|---|---|---|---|
| `offline.reconnecting` | Шукаємо станцію в локальній мережі… | Looking for the station on the local network… | Банер, повторне виявлення |
| `offline.cloudFallback` | Локально станція не відповідає — дані з хмари, оновлюються рідше. | No local connection — data comes from the cloud and updates less often. | Банер, інфо |
| `offline.none.title` | Немає зв'язку зі станцією | No connection to the station | Банер, попередження |
| `offline.none.body` | Ні локально, ні через хмару. Останні дані о {time}. Пробуємо знову. | Neither local nor cloud. Last data at {time}. Retrying. | |
| `offline.none.hint` | Якщо немає світла, роутер теж міг вимкнутись. | If the grid is off, your router may be off too. | Лише коли останній відомий стан — без світла |
| `offline.noInternet.cloud` | Немає інтернету — хмара Marstek недоступна. Локальні дані надходять. | No internet — Marstek cloud is unreachable. Local data is still coming in. | |
| `offline.restored.announce` | Зв'язок зі станцією відновлено. | Connection to the station restored. | Live region |

### 13.4 Загальні помилки додатка

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `error.cloud.login.credentials` | Пошта або пароль не підходять. Перевірте їх — це ті самі дані, що в застосунку Marstek на телефоні. | The email or password doesn't match. Use the same details as in the Marstek phone app. | Під формою |
| `error.cloud.login.unreachable` | Сервер Marstek не відповідає. Перевірте інтернет або спробуйте за кілька хвилин. | Marstek's server isn't responding. Check your internet or try again in a few minutes. | |
| `error.cloud.login.changed` | Не вдалося увійти: схоже, Marstek змінив спосіб входу. Перевірте, чи є оновлення додатка. | Couldn't sign in: Marstek may have changed how sign-in works. Check for an app update. | Неофіційний API зламався |
| `error.station.notFound` | Станцію не знайдено. Перевірте, чи вона ввімкнена, у тому самому Wi-Fi, і чи ввімкнено локальний API. | Station not found. Check that it's on, on the same Wi-Fi, and that the local API is turned on. | + `common.action.howTo` |
| `error.station.localApiOff` | Станція є в мережі, але локальний API вимкнено. Його вмикають на самій станції. | The station is on the network, but its local API is off. It's turned on in the station itself. | + `help.localApi.*` |
| `error.station.unsupported` | Модель {modelCode} ще не перевірена. Деякі показники можуть бути неточними або не з'явитися. | Model {modelCode} hasn't been tested. Some values may be inaccurate or missing. | Не блокує додавання |
| `error.export.failed` | Не вдалося зберегти файл: {reason}. Виберіть іншу папку. | Couldn't save the file: {reason}. Choose another folder. | `reason` — з ОС, локалізований |
| `error.storage` | Не вдалося записати історію на диск. Нові дані поки не зберігаються. Перевірте вільне місце на диску. | Couldn't write history to disk. New data isn't being saved. Check free disk space. | Банер, постійний |
| `error.generic` | Щось пішло не так у додатку — дані станції це не зачепило. Спробуйте ще раз. | Something went wrong in the app — station data isn't affected. Try again. | Лише як запасний варіант |
| `error.reference` | Код для звіту про помилку: {ref} | Reference for bug reports: {ref} | Дрібно, копіюється |

### 13.5 Картка пропуску, LIKELY_DEAD і неточні тривалості

**Картка «Поки додаток не працював»** — над кільцями на Огляді, пунктирна рамка, закривається ×; для того самого пропуску більше не показується. Вона замінює тости для подій, відновлених заднім числом (flows D5). Тіло складається з речень, які стосуються цього пропуску, у порядку нижче; кожне речення — окремий ключ, без склеювання слів у коді.

| Ключ | UK | EN | Коли |
|---|---|---|---|
| `gapCard.title` | Поки додаток не працював · {start}–{end} | While the app wasn't running · {start}–{end} | Завжди. `{start}` з датою, якщо не сьогодні |
| `gapCard.stationOut` | Станція, ймовірно, розрядилась ≈ о {time} (заряд перед цим {soc}%). | The station probably ran out at ≈ {time} (charge before that {soc}%). | Умова LIKELY_DEAD на останньому записі |
| `gapCard.gridOff` | Світло зникло о {time}{approx, select, true { (приблизно)} other {}}. | The grid went off at {time}{approx, select, true { (approximately)} other {}}. | Відключення почалось у пропуску або перед ним |
| `gapCard.gridBack` | Світло з'явилось о {time}. | The grid came back at {time}. | |
| `gapCard.stillOff` | Світла досі немає — без світла вже {duration}. | The grid is still off — {duration} so far. | Відключення триває на момент старту |
| `gapCard.outages` | За цей час — {count, plural, one {# відключення} few {# відключення} many {# відключень} other {# відключення}} (з хмари). | {count, plural, one {# outage} other {# outages}} in this time (from the cloud). | Хмара віддала історію |
| `gapCard.cloudFilled` | Дані за цей час дозавантажено з хмари. | Data for this time was filled in from the cloud. | Повне дозавантаження |
| `gapCard.cloudPartial` | Хмара віддала дані лише частково — за {duration} даних немає. | The cloud returned only part of the data — {duration} is missing. | |
| `gapCard.cloudNone` | Хмара за цей час даних не віддала. | The cloud had no data for this time. | Хмара підключена, але порожньо |
| `gapCard.noCloud` | Хмару не підключено, тож що було зі світлом у цей час, невідомо. | The cloud isn't connected, so grid status for this time is unknown. | + `gapCard.action.connectCloud` |
| `gapCard.action.show` | Показати у Відключеннях | Show in Outages | |
| `gapCard.action.connectCloud` | Підключити хмару | Connect cloud | |
| `gapCard.dismiss.a11y` | Закрити картку | Dismiss | |

Приклад з макета 09: «Поки додаток не працював · 01:20–02:40» + `gapCard.stationOut` + `gapCard.gridBack` + `gapCard.cloudNone`.

Причини пропуску на графіках (flows D4), доповнюють `gap.label.*` (§3.1): `gap.label.pcSleep` «ПК спав» / «PC was asleep» · `gap.label.noLink` «немає зв'язку» / «no connection» · `gap.label.likelyOut` «ймовірно, розрядилась» / «probably ran out» · `gap.label.update` «оновлення додатка» / «app update» · `gap.label.archived` (§11.3). `chart.observedSince` «спостереження з {date}» / «monitoring since {date}» — дні до першого запису (не пропуск).

**Банер LIKELY_DEAD**, виявлено наживо (червонувата рамка, один банер за раз):

| Ключ | UK | EN |
|---|---|---|
| `link.likelyOut.title` | Станція, ймовірно, розрядилась | The station probably ran out |
| `link.likelyOut.body` | {time} · заряд перед цим {soc}%. Світла немає з {gridOffSince}. Щойно з'явиться світло, станція ввімкнеться сама. | {time} · charge before that {soc}%. The grid has been off since {gridOffSince}. The station will turn back on by itself when the grid returns. |
| `overview.ring.soc.lastAt` | о {time} | at {time} |
| `overview.ring.soc.likelyOut.sub` | ймовірно, розрядилась ≈ о {time} | probably ran out at ≈ {time} |
| `overview.side.grid.offAtLeast` | немає з {time} · ≥ {duration} | off since {time} · ≥ {duration} |
| `overview.side.grid.frozen` | на {time}, далі невідомо | as of {time}; unknown after |
| `overview.side.grid.unknownLast` | невідомо · останнє: {lastState} | unknown · last: {lastState} |
| `overview.side.source.connecting` | {source} · підключаємось… | {source} · connecting… |
| `overview.side.source.lastAt` | останні дані о {time} | last data at {time} |

«Щойно з'явиться світло, станція ввімкнеться сама» — **факт для розробника**: перевірити на Venus E 3.0, що станція після глибокого розряду вмикається сама, коли повертається мережа. Якщо ні, речення прибрати.

**Неточні тривалості** (≥ — нижня межа, ≈ — оцінка) у таблиці Відключень і статистиці:

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `outages.duration.atLeast` | ≥ {duration} | ≥ {duration} | |
| `outages.duration.approx` | ≈ {duration} | ≈ {duration} | |
| `outages.table.end.linkLost` | зв'язок втрачено | connection lost | Колонка «Кінець» |
| `outages.table.asOf` | на {time} | as of {time} | Під «≥ …»: момент, до якого тривалість відома |
| `outages.note.frozen` | тривалість заморожена до відновлення зв'язку | duration frozen until the connection is back | |
| `outages.stats.footnote` | {count, plural, one {# відключення} few {# відключення} many {# відключень} other {# відключення}} з неточною тривалістю — у середній і найдовшій пораховано за нижньою межею. | {count, plural, one {# outage has} other {# outages have}} an uncertain duration — average and longest use the lower bound. | |
| `outages.chart.day.noData` | немає даних | no data | Штрихований стовпчик на весь день |
| `outages.chart.day.zero` | 0 відключень | 0 outages | Тонка риска на нулі; тултіп |
| `overview.chart.band.atLeast` | без світла ≥ {duration} | grid off ≥ {duration} | Смуга, що обривається на останньому вимірі |
| `overview.chart.resyncing` | Відновлюємо історію з хмари… | Restoring history from the cloud… | Тонкий індикатор над графіком |

**Банери зв'язку з flows §2.2** (діагноз NO_LINK; одна пара — заголовок банера і порада):

| Ключ | UK | EN |
|---|---|---|
| `link.connecting` | = `offline.reconnecting` | |
| `link.dx.pcOffline` | Цей комп'ютер не в мережі — ні станція, ні хмара не досяжні. Перевірте Wi-Fi або кабель. | This PC is offline — neither the station nor the cloud can be reached. Check Wi-Fi or the cable. |
| `link.dx.deviceSilentInCloud` | Станція не звітує в хмару з {time}. Схоже, вимкнена станція або роутер. | The station hasn't reported to the cloud since {time}. The station or router may be off. |
| `link.dx.lanSilentNoCloud` | Локальний API станції не відповідає з {time}. Це відома проблема прошивки — зазвичай минає сама. Хмара як запасний канал допоможе не втрачати дані. | The station's local API hasn't responded since {time}. It's a known firmware issue and usually clears up on its own. Adding the cloud as a backup keeps data flowing. |
| `link.dx.cloudUnreachable` | Хмара Marstek недоступна. Пробуємо знову. | The Marstek cloud is unreachable. Retrying. |
| `link.dx.cloudAuthExpired` | = `notify.cloudSession` (заголовок + тіло) + кнопка `cloud.login.action` | |
| `link.dx.cloudIncompatible` | Хмара Marstek змінила формат даних — потрібне оновлення додатка. | The Marstek cloud changed its data format — the app needs an update. |
| `link.noSources` | У цієї станції не лишилось підключень. Додайте локальне або хмарне, щоб знову бачити дані. | This station has no connections left. Add a local or cloud connection to see data again. |
| `link.noSources.action` | Додати підключення | Add connection |
| `tray.tooltip.connecting` | {station} · {model}\nПідключаємось… | {station} · {model}\nConnecting… |

### 13.6 Оновлення і бриф SmartScreen

План 3 (рішення власника, замість flows §6 «тосту немає ніколи»): один тост на кожну нову версію, тихий у тихі години й «Не турбувати»; клік відкриває Налаштування → «Про програму». Там рядок оновлення: «Перевірити зараз», «Доступна версія {version}.» з «Оновити» і «Завантажити». `settings.about.checkUpdates` не показується (до першої перевірки в рядку лише кнопка, версію називає рядок вище). `settings.about.update.failed.v1` замінює `settings.about.update.failed` («немає зв'язку» не підходить для ліміту GitHub чи 5xx).

«Оновити» (план 3, Task 5) — одне натискання: завантаження → перевірка SHA-256 → копія бази → тихий інсталятор, програма закривається. Рядок показує фазу (`update.downloading` / `update.verifying` / `update.backingUp` / `update.starting`) зі смугою прогресу. Помилка — причина (`update.failed.download` / `.integrity` / `.backup` / `.start`) під рядком, кнопки «Оновити» (ще раз) і «Завантажити» лишаються. `update.ready`, `update.ready.install` і `update.ready.later` у цьому потоці не показуються: проміжного кроку «Встановити» немає.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `update.available` | = `settings.about.update.available` | | |
| `notify.update.title` | Доступна версія {version} | Version {version} is available | Тост, раз на версію |
| `notify.update.body` | Натисніть, щоб оновити | Click to update | Клік → «Про програму» |
| `update.download` | Завантажити | Download | |
| `update.skip` | Пропустити цю версію | Skip this version | |
| `update.releaseNotes` | Що нового — на GitHub | What's new — on GitHub | |
| `update.downloading` | Завантажуємо {version}… {percent}% | Downloading {version}… {percent}% | |
| `update.verifying` | Перевіряємо файл… | Verifying the file… | |
| `update.backingUp` | Зберігаємо копію бази даних… | Backing up the database… | План 3, Task 5: фаза «Оновити» після перевірки |
| `update.starting` | Запускаємо інсталятор. Програма закриється й відкриється знову. | Starting the installer. The app will close and open again. | План 3, Task 5: остання фаза |
| `update.ready` | Версію {version} завантажено й перевірено. | Version {version} is downloaded and verified. | |
| `update.ready.install` | Встановити | Install | → бриф SmartScreen |
| `update.ready.later` | Пізніше | Later | |
| `update.failed.download` | Не вдалося завантажити оновлення. Спробуйте ще раз або завантажте зі сторінки релізу. | Couldn't download the update. Try again or download it from the release page. | |
| `update.failed.integrity` | Завантажений файл не збігся з опублікованою сумою SHA-256, тому його видалено. Спробуйте завантажити ще раз. | The downloaded file didn't match the published SHA-256 checksum, so it was deleted. Try downloading again. | Ніколи не пропонувати «встановити все одно» |
| `update.failed.backup` | Не вдалося зберегти копію бази даних, тому оновлення не встановлено. Спробуйте ще раз або завантажте зі сторінки релізу. | Couldn't back up the database, so the update wasn't installed. Try again or download it from the release page. | План 3, Task 5: без копії інсталятор не запускається |
| `update.failed.start` | Не вдалося запустити інсталятор. Спробуйте ще раз або завантажте зі сторінки релізу. | Couldn't start the installer. Try again or download it from the release page. | План 3, Task 5 |
| `update.releasePage` | Відкрити сторінку релізу | Open release page | |
| `update.check.failed.rate` | GitHub тимчасово обмежив перевірки. Спробуємо знову о {time}. | GitHub is temporarily limiting checks. We'll try again at {time}. | |
| `update.prerelease` | Тестові версії | Test versions | Перемикач у «Про програму» |
| `update.prerelease.help` | Пропонувати й пререлізи. Вони можуть бути нестабільні. | Also offer pre-releases. They may be unstable. | |

**Бриф перед запуском інсталятора** (`SMARTSCREEN_BRIEF`) стосується лише ручного завантаження інсталятора зі сторінки релізу (кнопка «Завантажити», браузер): такий файл має позначку «з Інтернету» (Mark-of-the-Web), і Windows SmartScreen може його зупинити. В оновленні одним натисканням («Оновити», план 3 Task 5) бриф не показується: додаток завантажує файл сам, без позначки «з Інтернету», перевіряє SHA-256 і запускає інсталятор напряму, тож вікна SmartScreen немає (власнику: підтвердити під час приймального встановлення). Назви кнопок Windows («Докладніше», «Виконати все одно», «Не виконувати») — це текст самої ОС. Їх беремо такими, як їх показує Windows **мовою інтерфейсу Windows**, а не мовою додатка. Розробнику: звірити з реальним вікном українського й англійського Windows 11.

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `smartscreen.title` | Встановити версію {version} | Install version {version} | |
| `smartscreen.intro` | Windows покаже синє вікно-попередження. Це очікувано — ось як пройти його за два кліки. | Windows will show a blue warning window. That's expected — here's how to get past it in two clicks. | |
| `smartscreen.step1` | Натисніть «{moreInfo}» — з'явиться кнопка запуску. | Click “{moreInfo}” — a button to run the installer appears. | `moreInfo` = «Докладніше» / “More info” |
| `smartscreen.step2` | Натисніть «{runAnyway}». | Click “{runAnyway}”. | `runAnyway` = «Виконати все одно» / “Run anyway” |
| `smartscreen.why` | Чому так: інсталятор не підписаний платним сертифікатом. Файл уже перевірено — сума SHA-256 збігається з опублікованою на GitHub. | Why: the installer isn't signed with a paid certificate. The file has already been checked — its SHA-256 matches the one published on GitHub. | Лише якщо перевірка справді пройшла (стан `READY`) |
| `smartscreen.checksum` | SHA-256 {shortHash} ✓ | SHA-256 {shortHash} ✓ | `9f2c…a41e` |
| `smartscreen.duringOutage` | Оновлення займе близько хвилини. Зараз без світла: дані за цей час дозавантажаться з хмари. | The update takes about a minute. The grid is off right now: data for that minute will be filled in from the cloud. | `GRID_OFF` + хмара підключена |
| `smartscreen.duringOutage.noCloud` | Оновлення займе близько хвилини. Зараз без світла, а хмару не підключено — ця хвилина буде пропуском у даних. | The update takes about a minute. The grid is off and the cloud isn't connected — that minute will be a gap in the data. | |
| `smartscreen.back` | Назад | Back | |
| `smartscreen.install` | Встановити | Install | Основна; запускає інсталятор і закриває додаток |
| `smartscreen.illustration.a11y` | Схема вікна «Windows захистив ваш ПК»: спочатку посилання «{moreInfo}», потім кнопка «{runAnyway}». | Diagram of the “Windows protected your PC” window: first the “{moreInfo}” link, then the “{runAnyway}” button. | |

### 13.7 macOS: тексти `.mac`

План macOS-порту (Task 2). Текст, що називає Windows, трей чи брандмауер, має на Mac варіант під тим самим ключем + `.mac`; його вибирає `pk(key, platform)` (`frontend/src/i18n/platform.ts`) за `SettingsView.Platform` = `darwin`. Решта текстів однакова. На Mac не показуються `onboarding.dx.netPublic.*` (macOS не повідомляє про загальнодоступну мережу) і `onboarding.done.trayHint` (підказка про панель задач Windows 11). Дія блоку брандмауера на Mac відкриває налаштування «Локальна мережа», без UAC; її `pending` / `error` мають свої `.mac`. Mac-варіант ⓘ `info.settings.quiet.mac` — у §15. Тексти Go (`app/i18n`: тости, меню іконки) теж мають `.mac`; їх вибирає `i18n.PlatformKey` за `runtime.GOOS`. Ні `Windows`, ні трей, ні інсталятор не називає жоден текст, який бачить Mac (тести Go і frontend).

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `settings.general.autostart.mac` | Відкривати під час входу в систему | Open at login | Також DONE першого запуску |
| `settings.general.autostart.moveFirst.mac` | Спершу перетягніть Marstek Monitor у «Програми» | Move Marstek Monitor to Applications first | Лише Mac, без базового ключа. Помилка під перемикачем, коли програма запущена з dmg (`/Volumes/`) чи з AppTranslocation: Go відмовляє (`ErrAutostartMoveFirst`) |
| `settings.general.minimizeToTray.mac` | Згортати в рядок меню при закритті | Minimize to the menu bar when closed | Також DONE першого запуску |
| `settings.general.language.desc.mac` | За замовчуванням — мова macOS. | macOS language by default. | |
| `settings.general.language.system.mac` | Як у macOS | Same as macOS | |
| `settings.notify.test.desc.mac` | Перевірити, що macOS показує сповіщення (наприклад, що їх не приховує режим зосередження). | Check that macOS shows notifications (for example, that a Focus mode doesn't hide them). | |
| `settings.notify.test.sent.mac` | Надіслано. Не з'явилось? Відкрити налаштування сповіщень macOS | Sent. Didn't see it? Open macOS notification settings | Хвіст після «? » — кнопка |
| `settings.notify.osDisabled.mac` | Сповіщення для Marstek Monitor вимкнено в macOS. Відкрити налаштування macOS | Notifications for Marstek Monitor are turned off in macOS. Open macOS settings | Хвіст після «. » — кнопка |
| `settings.stations.trayShows.mac` | У рядку меню показується: {station} | Shown in the menu bar: {station} | |
| `addStation.name.help.mac` | Так станція підписана в рядку меню та сповіщеннях. | This name appears in the menu bar and notifications. | Не в плані порту: базовий текст називає трей |
| `help.localApi.firewall.mac` | Якщо macOS запитає дозвіл на доступ до локальної мережі, натисніть «Дозволити». | If macOS asks for access to the local network, click “Allow”. | |
| `onboarding.dx.firewall.title.mac` | Немає дозволу на локальну мережу | No local network permission | |
| `onboarding.dx.firewall.body.mac` | macOS не дозволяє Marstek Monitor шукати пристрої в локальній мережі. Увімкніть дозвіл у «Системних параметрах» → «Конфіденційність і безпека» → «Локальна мережа». | macOS doesn't let Marstek Monitor look for devices on the local network. Turn it on in System Settings → Privacy & Security → Local Network. | |
| `onboarding.dx.firewall.action.mac` | Відкрити налаштування | Open settings | Відкриває «Локальна мережа» |
| `onboarding.dx.firewall.pending.mac` | Відкриваю налаштування… | Opening settings… | Поки налаштування відкриваються |
| `onboarding.dx.firewall.error.mac` | Не вдалося відкрити налаштування. Відкрийте «Системні параметри» → «Конфіденційність і безпека» → «Локальна мережа». | Couldn't open settings. Open System Settings → Privacy & Security → Local Network. | Налаштування не відкрилися |
| `tray.firstMinimize.toast.title.mac` | Marstek Monitor працює в рядку меню | Marstek Monitor is running in the menu bar | Go (`app/i18n`), тост першого закриття вікна |
| `tray.firstMinimize.toast.body.mac` | І далі стежить за станцією. Щоб вийти, клацніть іконку → «Вийти». | It keeps watching the station. To quit, click the icon → Quit. | Go (`app/i18n`). На Mac меню відкривається звичайним кліком |
| `update.starting.mac` | Встановлюємо оновлення. Програма закриється й відкриється знову. | Installing the update. The app will close and open again. | Mac оновлюється без інсталятора (заміна .app) |
| `update.failed.start.mac` | Не вдалося встановити оновлення. Спробуйте ще раз або натисніть «Завантажити». | Couldn't install the update. Try again or click Download. | |
| `update.moveToApps.mac` | Щоб оновлюватись одним кліком, перетягніть Marstek Monitor у «Програми». | To update with one click, move Marstek Monitor to Applications. | Лише Mac, без базового ключа. Рядок під оновленням, коли є нова версія, а `CanInstall` = false (запуск з dmg чи з AppTranslocation); «Оновити» тоді не показується |

---

## 14. Рішення (підтверджено власником 2026-11-21)

| # | Питання | Рішення | Чому |
|---|---|---|---|
| 1 | «г» чи «год» у тривалостях, «кВт·г» чи «кВт·год» | **[вирішено власником]** **«год», «кВт·год»** («2 год 05 хв») | «г» за стандартом — грам; «год» — нормативне скорочення. +2 символи, у всі бюджети влазить. Якщо хочеться компактніше — у кільцях уже годинникова форма «7:30» |
| 2 | Десятковий роздільник в uk | **[вирішено власником]** **Кома** («3,7 з 5,12 кВт·год») через `Intl` | Українська норма; крапка в макетах — з англійського прототипу |
| 3 | Права колонка: «Мережа» → **«Світло»** | **[вирішено власником]** **«Світло»** | Одне поняття — одне слово; «мережа» лишається тільки для джерела енергії й не плутається з Wi-Fi |
| 4 | Трей: «вистачить на 7:30» → **«вистачить на 7 год 30 хв»** | **[вирішено власником]** **Коротка форма** | «7:30» поруч із часом доби читається як «до 7:30»; ліміт 127 символів дозволяє |
| 5 | English: «station» чи «battery» для пристрою | **[вирішено власником]** **station** | Одне слово на обидві мови, «Battery» зайнята вкладкою здоров'я |
| 6 | Формат часу в English | **[вирішено власником]** **24 год, en-GB** («21 Nov 14:05») | Аудиторія — Україна; альтернатива — брати формат з регіональних налаштувань Windows |
| 7 | Звертання «ви» | **[рішення райтера, не обговорювалось]** **«ви», кнопки інфінітивом** | Уже в макетах; спокійний, дорослий тон для домашнього пристрою |
| 8 | Плитка «Пошук у мережі» → «Пошук у локальній мережі» | **[вирішено власником]** **Так** | «Мережа» в цьому продукті = електромережа |
| 9 | Підпис кільця «1 раз розрядилась раніше» → «1 раз, ймовірно, розрядилась» (2 рядки) | **[рішення райтера, не обговорювалось]** **Так** (бюджет 22) | Значення «раніше за кінець відключення» видно з назви кільця «Покриття»; повне — в a11y |

## 15. Пояснення (ⓘ-підказки)

Маленька ⓘ поруч зі значенням або підписом; підказка відкривається наведенням, фокусом з клавіатури і кліком (Esc закриває). Правила тексту: 1–2 короткі речення, uk ≤ 160 символів; спершу — що це означає, далі — що робити або що все гаразд. Жодної згадки про хмару (v1 — лише LAN). Слова — з глосарію (§2). Аргументи підставляються так само, як у видимому рядку поруч.

Де ⓘ **не** ставимо: біля самоочевидних підписів («Заряд станції, %», колонки таблиць, назви вкладок), біля причин пропуску на графіку («ПК спав», «немає зв'язку» — підпис уже пояснює сам себе) і в тостах Windows.

**Кнопка ⓘ**

| Ключ | UK | EN | Примітка |
|---|---|---|---|
| `info.button.a11y` | Що означає «{label}» | What “{label}” means | Доступна назва кнопки; `{label}` — видимий підпис поруч («Покриття») |

**Спільні позначки** (Огляд, Відключення — одна підказка на знак, щоб пояснення не розходились)

| Ключ | UK | EN | Де |
|---|---|---|---|
| `info.common.atLeast` | «≥» значить «щонайменше». Частину часу даних немає — додаток не працював або станція не відповідала, — тож насправді могло бути й довше. | “≥” means at least. Part of the time has no data (the app wasn't running or the station didn't answer), so the real figure may be longer. | Будь-яка тривалість з «≥» |
| `info.common.approx` | «≈» значить «приблизно»: це сталося, коли даних не було (додаток не працював або станція не відповідала), тож точної хвилини ми не знаємо. Якщо відомо, коли дані зникли, замість «≈» показуємо проміжок «між … і …»: від останнього виміру перед пропуском до першого після нього. | “≈” means about. It happened during a gap in the data (the app wasn't running or the station didn't answer), so we don't know the exact minute. When we know when the data stopped, we show the window instead, “between … and …”: from the last reading before the gap to the first one after it. | «≈» біля часу події (не прогнозу) |
| `info.common.notAfter` | Світло повернулось, коли даних не було: додаток не працював або станція не відповідала. Показуємо найпізніший можливий час — могло бути й раніше. | The grid came back during a gap in the data, when the app wasn't running or the station didn't answer. This is the latest it could have been; it may have been earlier. | `outages.table.end.notAfter` |

**Огляд — кільця**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.overview.soc` | Скільки енергії зараз у батареї. Друге число — ємність нової батареї; з роками вона трохи меншає (див. вкладку «Батарея»). | How much energy the battery holds right now. The second number is what a new battery holds; it shrinks a little over the years (see the Battery tab). | Кільце «Заряд» |
| `info.overview.power` | Скільки ватів зараз заходить у батарею чи виходить з неї. «з {max} Вт макс.» — межа цієї моделі; кільце показує, яку частку від неї використано. | How many watts are going into or out of the battery right now. “of {max} W max” is this model's limit; the ring shows how much of it is in use. | Кільце «Потужність» |
| `info.overview.toEmpty` | Стільки приблизно ще пропрацює станція, якщо навантаження не зміниться. Рахуємо до {floor}%: нижче станція не опускається, щоб берегти батарею. | Roughly how long the station will keep going if the load stays the same. We count down to {floor}%: the station stops there to protect the battery. | «До розряду» |
| `info.overview.toFull` | Стільки приблизно лишилось до 100%, якщо заряджати з тією ж потужністю. Після 90% станція заряджається повільніше, ми це врахували. | Roughly how long until 100% if charging carries on at this rate. Above 90% the station charges more slowly, and we allow for that. | «До повного» |
| `info.overview.lasts` | На скільки вистачить заряду, якщо світло зникне зараз. Беремо, скільки ви в середньому витрачали під час відключень за останні 30 днів. | How long the charge would last if the grid went off now, based on how much you used on average during outages in the last 30 days. | «Вистачить на» |
| `info.overview.lasts.noData` | Прогноз з'явиться після першого відключення, довшого за 15 хвилин: тоді ми знатимемо, скільки ви зазвичай витрачаєте без світла. | You'll see an estimate after your first outage longer than 15 minutes, once we know how much you usually use without power. | «ще немає даних» |
| `info.overview.calculating` | Прогноз з'явиться приблизно за хвилину. Спершу збираємо кілька вимірів потужності, щоб число не стрибало. | The estimate shows up in about a minute. We first collect a few power readings so the number doesn't jump around. | «рахуємо…» на будь-якому кільці часу |
| `info.overview.offToday` | Скільки часу від півночі не було світла і скільки разів воно зникало. Перебоїв, коротших за 30 секунд, не рахуємо. | How long the grid has been off since midnight, and how many times. Blips shorter than 30 seconds don't count. | «Без світла сьогодні» |
| `info.overview.offToday.gaps` | Частину дня даних немає: комп'ютер був вимкнений чи спав або станція не відповідала. Нічого не зламалось — тому й час із «≥». | Part of the day has no data: the PC was off or asleep, or the station didn't respond. Nothing is broken; that's just why the time shows “≥”. | «є пропуски в даних» (замість `info.overview.offToday`) |

**Огляд — права колонка, графік, картка пропуску**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.overview.grid` | Чи є зараз світло. Ми бачимо це з того, звідки станція бере енергію і куди її віддає. Час — скільки це вже триває. | Whether the grid is on right now. We tell from where the station draws power and where it sends it. The time is how long it's been that way. | «Світло» |
| `info.overview.grid.unknown` | Від станції немає свіжих даних, або з них не видно, чи є світло. Щойно станція відповість, тут знову буде «є» чи «немає». | We have no fresh data from the station, or it doesn't tell us whether the grid is on. As soon as the station answers, this will show on or off again. | «невідомо», «далі невідомо» |
| `info.overview.prevOutage` | Останнє відключення, що вже скінчилось. «≈» — початок відомий лише приблизно; «не пізніше» — світло повернулось, коли даних не було. | The most recent outage that's over. “≈” means we only know roughly when it started; “not after” means the grid came back during a gap in the data. | «Попереднє відключення» |
| `info.overview.source` | Звідки беруться дані (LAN — ваш Wi-Fi) і наскільки вони свіжі. Якщо станція довго мовчить, числа сірішають і з'являється «Дані застарілі». | Where the data comes from (LAN, your Wi-Fi) and how fresh it is. If the station goes quiet for a while, the numbers turn grey and “Data is out of date” appears. | «Джерело даних» |
| `info.overview.chart.outage` | Сіра смуга — час без світла. Підпис над нею — скільки воно тривало. | A grey band marks time without power. The label above it shows how long it lasted. | Легенда «без світла» |
| `info.overview.chart.noData` | Пунктир — тут немає даних: комп'ютер був вимкнений чи спав, роутер вимкнувся без світла або станція, ймовірно, розрядилась. Підпис підкаже, що саме. | Dashed area: no data. The PC was off or asleep, the router went down with the power, or the station probably ran out. The label tells you which. | Легенда «немає даних» |
| `info.overview.chart.floor` | Лінія — мінімальний заряд станції ({floor}%): нижче станція не розряджається, щоб берегти батарею. Змінити: Налаштування → Станції. | This line is the station minimum ({floor}%): the station doesn't discharge below it, to protect the battery. You can change it in Settings → Stations. | Легенда «мінімальний заряд станції» |
| `info.overview.gapCard` | Додаток записує, лише коли працює. Тут — що нам вдалося з'ясувати про час, коли він не працював. Картку можна закрити: усе є у «Відключеннях». | The app only records while it's running. Here's what we could piece together about the time it wasn't. You can close this card; it's all in Outages too. | Заголовок `gapCard.title` |

**Відключення**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.outages.grid` | Скільки часу за період світло було, у відсотках. Рахуємо лише час, коли є дані: пропуски не йдуть ні в «є», ні в «немає». | The share of the period you had grid power. We only count time with data; gaps count as neither on nor off. | Кільце «Світло» |
| `info.outages.coverage` | Скільки відключень станція витримала до кінця, не розрядившись. Ті, що ще тривають, не рахуються. | How many outages the station got through without running out. Outages still going on aren't counted. | Кільце «Покриття» |
| `info.outages.gapRow` | За частину цього відключення даних немає (додаток не працював або станція не відповідала), тож тут лише відомий відрізок. Могло тривати довше. | Part of this outage has no data (the app wasn't running or the station didn't answer), so this is only the part we saw. It may have lasted longer. | Примітка «є пропуск у даних» |
| `info.outages.startedWhileOff` | Світло зникло, поки комп'ютер був вимкнений чи спав. Додаток помітив це вже після запуску, тому початок приблизний. | The grid went off while the PC was off or asleep. The app only noticed once it started, so the start time is approximate. | Примітка «почалось, поки додаток не працював» |
| `info.outages.linkLost` | Під час відключення станція перестала відповідати. Тривалість поки що до останнього відомого моменту — вона оновиться, щойно станція озветься. | The station stopped responding during the outage. The duration runs to the last moment we know of and will update once the station is back. | «зв'язок втрачено», «тривалість заморожена…» |
| `info.outages.stationOut` | Під час відключення станція замовкла, коли заряду лишалось мало. Ймовірно, вона розрядилась. | During the outage the station went quiet while its charge was low. It most likely ran out. | Примітка «станція, ймовірно, розрядилась» |
| `info.outages.noDataDay` | За заштрихований день даних немає зовсім, тож ми не знаємо, чи було світло. Це не означає, що відключень не було. | There's no data at all for a hatched day, so we can't tell whether the grid was on. It doesn't mean there were no outages. | Заштрихований стовпчик, легенда графіка |

**Батарея**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.battery.health` | Скільки енергії батарея вміщує зараз порівняно з новою. Оцінюємо за довгими розрядами, і з часом оцінка точнішає. Нижче 70% кільце стає червоним. | How much the battery holds now compared with when it was new. We estimate it from long discharges, and it gets more accurate over time. Below 70% the ring turns red. | Кільце «Здоров'я» |
| `info.battery.health.pending` | Щоб оцінити здоров'я, потрібен один довгий розряд — коли станція віддасть щонайменше половину заряду. Зазвичай це перше довге відключення. | We need one long discharge first, where the station uses at least half its charge. That's usually your first long outage. | «ще рахуємо» |
| `info.battery.health.prelim` | Поки що оцінка спирається на один-два розряди, тож ще може змінитись. Точнішою вона стане, коли за місяць їх буде три або більше. | So far this is based on one or two discharges, so it may still change. It firms up once there are three or more in a month. | «попередня оцінка» |
| `info.battery.cycles` | Батарея розрахована на понад {rated} циклів при розряді на {dod} %. Один цикл тут — це віддача ~{cycleKWh} кВт·год. Рахуємо самі, тому «≈». | The maker rates it for over {rated} cycles at {dod}% depth of discharge. One cycle here is ~{cycleKWh} kWh given out. We count these ourselves, hence “≈”. | Кільце «Ресурс циклів»; {dod} = RatedDoD × 100, {cycleKWh} = RatedDoD × NominalWh (Venus E 3.0: 90, «4,6») |

**Енергія**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.energy.charged` | Скільки енергії батарея отримала за цей період. Рахуємо за вимірами потужності, які беремо кожні кілька секунд. | How much energy went into the battery in this period. We add it up from power readings taken every few seconds. | «Заряджено» |
| `info.energy.discharged` | Скільки енергії батарея віддала за цей період і скільки з неї припало на відключення. | How much energy the battery gave out in this period, and how much of it during outages. | «Віддано» |
| `info.energy.efficiency` | Скільки з отриманої енергії батарея віддала назад; решта йде в тепло. За короткий період число приблизне, бо заряд на початку й наприкінці різний. | How much of the energy put in the battery gave back; the rest is lost as heat. Over a short period it's only rough, because the charge at the start and end differs. | «ККД циклу» |
| `info.energy.incomplete` | За частину періоду даних немає (комп'ютер був вимкнений чи спав або станція не відповідала), тож насправді суми більші. | Part of this period has no data (PC off or asleep, or the station didn't respond), so the real totals are higher. | `energy.incomplete` |

**Помилки**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.errors.level` | Інфо — просто запис, нічого робити не треба. Попередження — варто прочитати пораду. Помилка — станція повідомила про збій. | Info is just a note, nothing to do. Warning: worth reading the advice. Error: the station reported a fault. | Заголовок колонки «Рівень» |
| `info.errors.new` | Попередження й помилки, що з'явились, відколи ви востаннє відкривали цю вкладку. Відкрийте її — і лічильник обнулиться. | Warnings and errors since you last opened this tab. Opening it clears the count. | Бейдж «нові» на вкладці, рядок «Помилки» на Огляді |

**Налаштування**

| Ключ | UK | EN | Біля |
|---|---|---|---|
| `info.settings.poll` | Як часто питати станцію, коли щось відбувається. Коли світло є й нічого не змінюється, питаємо раз на 20 с — або рідше, якщо вибраний інтервал довший. | How often to check the station while something's happening. When the grid is on and nothing changes, we check every 20 s, or less often if your interval is longer, to go easy on the station. | «Опитування локально» |
| `info.settings.quiet` | Тиха година вмикається щоночі за розкладом. «Не турбувати» — разово, на вибраний час, з меню іконки в треї. «Низький заряд» приходить в обох випадках. | Quiet hours switch on every night on a schedule. Do not disturb is a one-off for a set time, from the tray icon menu. Low charge alerts come through either way. | «Тиха година» |
| `info.settings.quiet.mac` | Тиха година вмикається щоночі за розкладом. «Не турбувати» — разово, на вибраний час, кліком по іконці в рядку меню. «Низький заряд» приходить в обох випадках. | Quiet hours switch on every night on a schedule. Do not disturb is a one-off for a set time, from the menu bar icon. Low charge alerts come through either way. | «Тиха година», на Mac (§13.7) |
| `info.settings.lowThreshold` | Коли заряд опуститься до цього рівня, ви отримаєте сповіщення. | You'll get a notification when charge drops to this level. | «Поріг сповіщення» |
| `info.settings.noLinkAfter` | Сповіщення, якщо станція мовчить довше за вибраний час. Без світла роутер часто теж вимикається, тому за замовчуванням це вимкнено. | Lets you know if the station stays silent longer than this. Routers often go down in an outage too, so this is off by default. | «Немає зв'язку зі станцією» + «Після» |
| `info.settings.retention` | Скільки зберігати на цьому комп'ютері відключення, події та історію заряду. Старіші записи ми видаляємо, але лічильник циклів не зменшується. | How long to keep outages, events and charge history on this PC. Older records are removed, but the cycle count never goes down. | «Зберігати історію» |
| `info.settings.export` | Три файли CSV за вибраний період: відключення, події та заряд. Їх можна відкрити в Excel чи Google Таблицях. | Three CSV files for the period you pick: outages, events and charge. You can open them in Excel or Google Sheets. | «Експортувати в CSV» |

Факти, на яких стоять підказки (перевірено в коді 2026-11-21): дебаунс світла 30 с (`core/monitor` `gridDebounce`); прогрів прогнозу 60 с і сповільнення заряду від 90% (`core/forecast`); «Вистачить на» — відключення ≥ 15 хв за 30 днів (там же); нижня межа {floor} і ресурс {rated} — з профілю моделі (`core/model/profile.go`: 10%, 6000 при RatedDoD 0,9; один цикл = RatedDoD × NominalWh, Ruling 19); опитування в спокої — max(20 с, інтервал) (план 2, Task 2b); поріг здоров'я 70% і «попередня оцінка» при < 3 розрядах за місяць (план 2, Task 14–15); «нові» = попередження й помилки після `journal.seen_at` (Task 10). Якщо щось із цього зміниться — міняти й підказку.

**Humanizer pass (2026-11-21).** Переписано 44 з 45 текстів (`info.button.a11y` без змін): дієслова замість іменникових ланцюжків і пасиву, «ми» як діючий, пропуски в даних без вигаданої причини («даних немає: додаток не працював або станція не відповідала»), причина — лише там, де вона відома, спокійне «нічого не зламалось» там, де значення лякає. Факти й числа не змінено; uk ≤ 160 символів. EN переписано окремо, не дослівно. Приклади:

| Ключ | Було | Стало |
|---|---|---|
| `info.overview.offToday.gaps` | Частину дня додаток не записував: …або не було зв'язку зі станцією. Це не збій. Тому час — «≥». | Частину дня даних немає: …або станція не відповідала. Нічого не зламалось — тому й час із «≥». |
| `info.overview.toEmpty` | Приблизно стільки станція ще працюватиме при теперішньому навантаженні. | Стільки приблизно ще пропрацює станція, якщо навантаження не зміниться. |
| `info.outages.stationOut` | …станція зникла зі зв'язку з низьким зарядом. Найімовірніше, вона розрядилась і вимкнулась. | …станція замовкла, коли заряду лишалось мало. Ймовірно, вона розрядилась. |
| `info.errors.new` | …що з'явились після вашого останнього перегляду цієї вкладки. Коли ви її відкриєте, лічильник обнулиться. | …що з'явились, відколи ви востаннє відкривали цю вкладку. Відкрийте її — і лічильник обнулиться. |
| `info.settings.retention` | Відключення, події й історія заряду зберігаються на цьому комп'ютері стільки, скільки вибрано. | Скільки зберігати на цьому комп'ютері відключення, події та історію заряду. |

## 16. Переклад і перевірка

- Джерельна мова — **українська**; англійська — переклад. Англійські рядки цього документа — чернетка, яку має перечитати людина з рідною або вільною англійською перед публікацією на GitHub.
- Українські рядки має переглянути власник у самому додатку (а не в таблиці): кожен екран на найменшій ширині вікна.
- Перед перекладом нових екранів — прогін псевдолокалізації (+35% довжини), щоб знайти зашиті в коді рядки і тісні місця.
- Контекстні примітки до ключів (частина мови, бюджет, аргументи) — у колонках «Примітка» цього документа; frontend переносить їх у `_context.json` поруч із файлами повідомлень.
