import { sha256 } from 'k6/crypto';

export function generatePayload(size) {
  const chunk = 'a'.repeat(1024);
  let remaining = size;
  let parts = [];
  while (remaining > 0) {
    parts.push(chunk.substring(0, Math.min(1024, remaining)));
    remaining -= 1024;
  }
  const payload = parts.join('');
  const checksum = sha256(payload, 'hex');
  return { payload, checksum };
}

