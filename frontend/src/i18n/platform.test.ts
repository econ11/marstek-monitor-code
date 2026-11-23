import { expect, test } from 'vitest'
import { pk } from './platform'

test('pk picks the .mac key on darwin when the catalogue has one', () => {
  expect(pk('settings.general.autostart', 'darwin')).toBe('settings.general.autostart.mac')
  expect(pk('settings.general.autostart', 'windows')).toBe('settings.general.autostart')
  expect(pk('settings.general.autostart', '')).toBe('settings.general.autostart')
  expect(pk('no.such', 'darwin')).toBe('no.such')
})
