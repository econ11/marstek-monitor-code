import { uk } from './uk'

// macOS port: a text that names Windows, the tray or the firewall has a Mac
// variant under the same key + '.mac' (copy §13.7). Everything else is the
// same on both.

/** The platforms the app ships for (SettingsView.Platform = runtime.GOOS). */
export type Platform = 'windows' | 'darwin'

/** pk is the catalogue key for the platform: key + '.mac' on darwin when the
 *  catalogue has that variant, else key. */
export function pk(key: string, platform: string): string {
  const mac = key + '.mac'
  return platform === 'darwin' && mac in uk ? mac : key
}
