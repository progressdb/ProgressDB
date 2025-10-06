import { sha256 } from 'k6/crypto';

const MAX_PAYLOAD_SIZE = 10 * 1024; // 10kb

export function generatePayload(size) {
  const actualSize = Math.min(size, MAX_PAYLOAD_SIZE);
  const payload = 'a'.repeat(actualSize);
  const checksum = sha256(payload, 'hex');
  return { payload, checksum };
}
