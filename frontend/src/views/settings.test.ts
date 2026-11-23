import { expect, test } from 'vitest'
import { actionHTML, autostartErrorKey, progressHTML } from './settings'

// Fix round 1: a disabled update button («Перевірити зараз» while the check
// runs) is aria-disabled, not disabled, so the keyboard focus stays on it.
test('a disabled action button keeps its focus', () => {
  const off = actionHTML('update', { act: 'check', label: 'Перевірити зараз', disabled: true })
  expect(off).toContain('aria-disabled="true"')
  expect(off).not.toMatch(/\sdisabled[\s>]/)
  expect(off).toContain('data-focus="update:check"')
  const on = actionHTML('update', { act: 'install', label: 'Оновити', primary: true })
  expect(on).not.toContain('disabled')
  expect(on).toContain('btn--primary')
})

// Task 5: the one-click update's progress bar, named by the state line.
test('the update progress bar', () => {
  expect(progressHTML({ id: 'update', label: '', control: 'actions' })).toBe('')
  const bar = progressHTML({ id: 'update', label: 'Завантажуємо 1.2.0… 42%', control: 'actions', progress: 0.42 })
  expect(bar).toContain('role="progressbar"')
  expect(bar).toContain('aria-labelledby="o-update"')
  expect(bar).toContain('aria-valuenow="42"')
  expect(bar).toContain('width:42%')
  expect(progressHTML({ id: 'update', label: '', control: 'actions', progress: 3 })).toContain('aria-valuenow="100"')
})

// macOS port, Task 6: «Open at login» from the dmg or a translocated copy is
// refused by Go (ErrAutostartMoveFirst) and says to move the app first.
test('autostart: move the app first on macOS', () => {
  expect(autostartErrorKey(new Error('autostart.moveFirst: move the app to Applications first'))).toBe('settings.general.autostart.moveFirst.mac')
  expect(autostartErrorKey('autostart.moveFirst: move the app to Applications first')).toBe('settings.general.autostart.moveFirst.mac')
  expect(autostartErrorKey(new Error('registry unreadable'))).toBe('settings.save.error')
})
