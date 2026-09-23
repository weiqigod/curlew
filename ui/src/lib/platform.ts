export function executableName(platform: string, base: string): string {
  if (platform === 'win32' && !base.toLowerCase().endsWith('.exe')) {
    return `${base}.exe`;
  }
  return base;
}