import { useEffect, useState } from 'react';
import { useProgressClient } from './client';
import type { MessageCreateRequestType, MessageUpdateRequestType, PaginationResponseType, MessagesListResponseType, KeyResponseType, MessageListQueryType, MessageType } from '@progressdb/js';

/**
 * Hook: list messages for a given thread.
 * Messages are returned in chronological order: [oldest → newest]
 * 
 * Pagination semantics for messages:
 * - before: load older messages (scroll up) → append to array
 * - after: load newer messages (scroll down) → append to array
 * 
 * @param threadKey thread key to list messages for
 * @param query optional pagination query parameters (limit, before, after, anchor, sort_by)
 * @param deps optional dependency array to re-run fetch
 */
export function useMessages(
  threadKey?: string, 
  query: MessageListQueryType = {}, 
  deps: any[] = []
) {
  const client = useProgressClient();
  const [messages, setMessages] = useState<MessageType[] | null>(null);
  const [pagination, setPagination] = useState<PaginationResponseType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);
  const [currentQuery, setCurrentQuery] = useState<MessageListQueryType>(query);

  const fetchMessages = async (customQuery?: MessageListQueryType) => {
    if (!threadKey) return;
    setLoading(true);
    setError(null);
    try {
      const queryToUse = customQuery || currentQuery;
      const res: MessagesListResponseType = await client.listThreadMessages(threadKey, queryToUse);
      setMessages(res.messages || []);
      setPagination(res.pagination || null);
      if (customQuery) {
        setCurrentQuery(customQuery);
      }
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (threadKey) fetchMessages();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadKey, ...deps]);

  const create = async (msg: MessageCreateRequestType) => {
    const created: KeyResponseType = await client.createThreadMessage(threadKey || '', msg);
    
    // Optimistically add to messages list if we have existing data
    if (messages && pagination) {
      // For messages, we need to fetch the created message since we only get the key
      const newMsgRes = await client.getThreadMessage(threadKey || '', created.key);
      // Messages are chronological [oldest → newest], so new message goes at end
      setMessages([...messages, newMsgRes.message]);
    } else {
      // Fallback to full refresh if no existing data
      await fetchMessages();
    }
    
    return created.key;
  };

  // Navigation helpers for Messages (MI): [oldest → newest] chronological
  // before = older messages, after = newer messages
  const loadOlder = async () => {
    // Load older messages (scroll up)
    if (pagination?.has_before && pagination.before_anchor) {
      await fetchMessages({ ...currentQuery, before: pagination.before_anchor });
    }
  };

  const loadNewer = async () => {
    // Load newer messages (scroll down)
    if (pagination?.has_after && pagination.after_anchor) {
      await fetchMessages({ ...currentQuery, after: pagination.after_anchor });
    }
  };

  const goToAnchor = async (anchor: string) => {
    await fetchMessages({ ...currentQuery, anchor });
  };

  const reset = async () => {
    // Reset to initial query state (clear pagination)
    await fetchMessages(query);
  };

  return { 
    messages, 
    pagination, 
    loading, 
    error, 
    refresh: fetchMessages, 
    reset,
    create,
    // Navigation helpers
    loadOlder,
    loadNewer,
    goToAnchor
  };
}