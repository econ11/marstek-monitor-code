/** stationLabel is how the UI names a station, the same rule as the tray
 *  (app/tray.go stationLabel): the user's name, else the model, else
 *  «Marstek». A station added before plan 2 has no name (Name ''). Every
 *  screen that shows a station's name uses this, never the raw Name. */
export function stationLabel(st: { Name?: string; Model?: string } | null | undefined): string {
  return st?.Name || st?.Model || 'Marstek'
}
