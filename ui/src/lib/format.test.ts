import { describe, expect, it } from 'vitest';
import { fileBasename, fmtBytes, fmtMs, fmtUs } from './format';

describe('fileBasename', () => {
  it.each([
    ['collections/users.yaml', 'users.yaml'],
    ['collections\\users.yaml', 'users.yaml'],
    ['collections\\nested/users.yaml', 'users.yaml'],
    ['users.yaml', 'users.yaml'],
    ['', ''],
  ])('extracts the display name from %j', (input, expected) => {
    expect(fileBasename(input)).toBe(expected);
  });
});

describe('fmtUs', () => {
  it('renders sub-millisecond values in µs', () => {
    expect(fmtUs(412)).toBe('412 µs');
    expect(fmtUs(0)).toBe('0 µs');
    expect(fmtUs(999)).toBe('999 µs');
  });

  it('renders >= 1000 µs as ms with one decimal', () => {
    expect(fmtUs(1000)).toBe('1.0 ms');
    expect(fmtUs(48200)).toBe('48.2 ms');
    expect(fmtUs(15400)).toBe('15.4 ms');
  });
});

describe('fmtMs', () => {
  it('renders sub-second values in ms', () => {
    expect(fmtMs(0)).toBe('0 ms');
    expect(fmtMs(184)).toBe('184 ms');
    expect(fmtMs(999)).toBe('999 ms');
  });

  it('renders seconds with one decimal', () => {
    expect(fmtMs(1000)).toBe('1.0 s');
    expect(fmtMs(4810)).toBe('4.8 s');
  });

  it('renders minutes + seconds', () => {
    expect(fmtMs(60_000)).toBe('1m 0s');
    expect(fmtMs(83_000)).toBe('1m 23s');
  });
});

describe('fmtBytes', () => {
  it('renders bytes below 1 KiB', () => {
    expect(fmtBytes(0)).toBe('0 B');
    expect(fmtBytes(512)).toBe('512 B');
    expect(fmtBytes(1023)).toBe('1023 B');
  });

  it('renders KB with one decimal, trimming .0', () => {
    expect(fmtBytes(1024)).toBe('1 KB');
    expect(fmtBytes(48211)).toBe('47.1 KB');
  });

  it('renders MB', () => {
    expect(fmtBytes(1048576)).toBe('1 MB');
    expect(fmtBytes(1572864)).toBe('1.5 MB');
  });
});
