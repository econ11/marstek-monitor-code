import { t } from '../lib/i18n'
import { icon } from './icons'
import { escapeHTML as esc } from './tooltip'

// The one loading state of every tab (owner feedback, Ruling 17): a spinner
// (static under prefers-reduced-motion), «Завантажуємо…», and, when the wait
// goes on (we never know how long it takes), «Це може зайняти кілька хвилин»
// fades in after a few seconds (CSS .loading__long), so a quick load does not
// warn about minutes.

/** spinnerHTML is the loader icon that turns. */
export const spinnerHTML = () => icon('loader', 'icon--spin')

/** loadingHTML is the status line; attrs go on the <p> (e.g. data-slot="loading"). */
export function loadingHTML(attrs = '', text = t('common.loading')): string {
  return `<p class="note loading" role="status"${attrs ? ' ' + attrs : ''}>${spinnerHTML()}<span class="loading__t">${esc(text)}</span>` +
    `<span class="loading__long">${esc(t('common.loading.long'))}</span></p>`
}

/** laterHTML: a tab that comes in a later version. A neutral note, no loader:
 *  nothing is loading there. */
export function laterHTML(): string {
  return `<p class="note" role="status">${icon('info')}<span>${esc(t('common.tab.later'))}</span></p>`
}
