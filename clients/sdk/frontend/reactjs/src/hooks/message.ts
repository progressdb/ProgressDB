import { useEffect, useState } from 'react';
import { useProgressClient } from './client';
import type { MessageUpdateRequest, MessageResponse } from '@progressdb/js';

/**
 * Hook: fetch/operate on a single message within a thread.
 * @param threadKey key of the thread containing the message
 * @param key message key
 */
export function useMessage(threadKey?: string, key?: string) {
  const client = useProgressClient();
  const [message, setMessage] = useState<any | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetchMessage = async () => {
    if (!key || !threadKey) return;
    setLoading(true);
    setError(null);
    try {
      const res = await client.getThreadMessage(threadKey, key);
      setMessage(res.message);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (key && threadKey) fetchMessage();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, threadKey]);

  const update = async (msg: MessageUpdateRequest) => {
    if (!key || !threadKey) throw new Error('threadKey and key required');
    await client.updateThreadMessage(threadKey, key, msg);
    // Refetch to get updated message
    await fetchMessage();
    const res: MessageResponse = await client.getThreadMessage(threadKey, key);
    return res.message;
  };

  const remove = async () => {
    if (!key || !threadKey) throw new Error('threadKey and key required');
    await client.deleteThreadMessage(threadKey, key);
    setMessage(null);
  };

  return { message, loading, error, refresh: fetchMessage, update, remove };
}