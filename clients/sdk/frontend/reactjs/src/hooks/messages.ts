import { useEffect, useState } from 'react';
import { useProgressClient } from './client';
import { validatePaginationQuery } from '../utils/validation';
import type { MessageCreateRequestType, MessageUpdateRequestType, PaginationResponseType, MessagesListResponseType, KeyResponseType, MessageListQueryType, MessageType, ApiErrorResponseType } from '@progressdb/js';

/**
 * Hook: list messages for a given thread.
 * Messages are returned in chronological order: [oldest → newest]
 * 
 * Pagination semantics for messages:
 * - before: load older messages (scroll up) → prepend to array
 * - after: load newer messages (scroll down) → append to array
 * - anchor: jump to specific position in message list
 * - limit: number of messages to return (1-100)
 * - sort_by: sort messages by 'created_ts' or 'updated_ts'
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
  const [error, setError] = useState<ApiErrorResponseType | null>(null);
  const [currentQuery, setCurrentQuery] = useState<MessageListQueryType>(query);

  const fetchMessages = async (customQuery?: MessageListQueryType) => {
    if (!threadKey) return;
    setLoading(true);
    setError(null);
    try {
      const queryToUse = customQuery || currentQuery;
      validatePaginationQuery(queryToUse);
      const res: MessagesListResponseType = await client.listThreadMessages(threadKey, queryToUse);
      setMessages(res.messages || []);
      setPagination(res.pagination || null);
      if (customQuery) {
        setCurrentQuery(customQuery);
      }
    } catch (err) {
      setError(err as ApiErrorResponseType);
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
    if (threadKey && pagination?.has_before && pagination.before_anchor) {
      const query = { 
        limit: currentQuery.limit, 
        sort_by: currentQuery.sort_by, 
        before: pagination.before_anchor 
      };
      const res = await client.listThreadMessages(threadKey, query);
      setMessages([...res.messages, ...(messages || [])]); // PREPEND older messages
      setPagination(res.pagination);
    }
  };

  const loadNewer = async () => {
    // Load newer messages (scroll down)
    if (threadKey && pagination?.has_after && pagination.after_anchor) {
      const query = { 
        limit: currentQuery.limit, 
        sort_by: currentQuery.sort_by, 
        after: pagination.after_anchor 
      };
      const res = await client.listThreadMessages(threadKey, query);
      setMessages([...(messages || []), ...res.messages]); // APPEND newer messages
      setPagination(res.pagination);
    }
  };

  const goToAnchor = async (anchor: string) => {
    const query = { 
      limit: currentQuery.limit, 
      sort_by: currentQuery.sort_by, 
      anchor 
    };
    await fetchMessages(query);
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