import { describe, expect, it } from 'vitest';
import { COLLECTION_PLATFORMS, catalogOS, collectionSupported } from './collectionSupport';

describe('collectionSupported', () => {
  it('sama dengan platform yang dipatok di server (internal/osquery SupportedPlatforms)', () => {
    expect(COLLECTION_PLATFORMS.join(',')).toBe('linux/x86_64,macos/x86_64,macos/arm64');
  });
  it('menerima Linux amd64 dan macOS Intel/Apple Silicon, menolak sisanya', () => {
    expect(collectionSupported('linux', 'x86_64')).toBe(true);
    expect(collectionSupported('macos', 'x86_64')).toBe(true);
    expect(collectionSupported('macos', 'arm64')).toBe(true);
    expect(collectionSupported('darwin', 'amd64')).toBe(true);
    expect(collectionSupported('linux', 'arm64')).toBe(false);
    expect(collectionSupported('windows', 'x86_64')).toBe(false);
    expect(collectionSupported('', '')).toBe(false);
    expect(collectionSupported(null, undefined)).toBe(false);
  });
  it('catalogOS memetakan darwin ke macos', () => {
    expect(catalogOS('darwin')).toBe('macos');
    expect(catalogOS('linux')).toBe('linux');
  });
});
