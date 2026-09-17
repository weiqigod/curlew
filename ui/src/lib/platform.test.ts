import { describe, expect, it } from 'vitest';
import { executableName } from './platform';

describe('executableName', () => {
  it.each([
    ['win32', 'curlew', 'curlew.exe'],
    ['win32', 'curlew.exe', 'curlew.exe'],
    ['win32', 'curlew.EXE', 'curlew.EXE'],
    ['linux', 'curlew', 'curlew'],
    ['darwin', 'curlew', 'curlew'],
  ])('maps %s %s to %s', (platform, base, expected) => {
    expect(executableName(platform, base)).toBe(expected);
  });
});