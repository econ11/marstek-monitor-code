import { expect, test } from 'vitest'
import { laterHTML, loadingHTML } from './loading'

test('loading: a spinning loader, the text and the long-wait line', () => {
  const html = loadingHTML('data-slot="loading"')
  expect(html).toContain('data-slot="loading"')
  expect(html).toContain('icon--spin')
  expect(html).toContain('Завантажуємо…')
  expect(html).toContain('Це може зайняти кілька хвилин')
})

test('a tab that comes later shows a note, not a loader', () => {
  const html = laterHTML()
  expect(html).toContain('Ця вкладка з&#39;явиться згодом')
  expect(html).not.toContain('loader')
})
