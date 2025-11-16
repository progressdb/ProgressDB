import type { Thread } from './thread';
import type { Message } from './message';
import type { PaginationResponse } from './pagination';

export type ThreadsListResponse = {
  threads: Thread[];
  pagination?: PaginationResponse;
};

export type MessagesListResponse = {
  thread?: string;
  messages: Message[];
  pagination?: PaginationResponse;
};