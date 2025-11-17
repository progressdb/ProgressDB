import type { ThreadType } from './thread';
import type { MessageType } from './message';
import type { PaginationResponseType } from './pagination';

export type KeyResponseType = {
  key: string;
};

export type CreateThreadResponseType = KeyResponseType;
export type CreateMessageResponseType = KeyResponseType;
export type UpdateThreadResponseType = KeyResponseType;
export type UpdateMessageResponseType = KeyResponseType;
export type DeleteThreadResponseType = KeyResponseType;
export type DeleteMessageResponseType = KeyResponseType;

// Standardized response wrapper pattern
export type ApiResponseType<T> = {
  data: T;
};

export type ThreadApiResponseType = ApiResponseType<ThreadType>;
export type MessageApiResponseType = ApiResponseType<MessageType>;
export type ThreadsListApiResponseType = ApiResponseType<ThreadType[]>;
export type MessagesListApiResponseType = ApiResponseType<MessageType[]>;

export type ThreadResponseType = {
  thread: ThreadType;
};

export type MessageResponseType = {
  message: MessageType;
};

export type ThreadsListResponseType = {
  threads: ThreadType[];
  pagination: PaginationResponseType;
};

export type MessagesListResponseType = {
  thread: string;
  messages: MessageType[];
  pagination: PaginationResponseType;
};

// Health check response types
export type HealthzResponseType = {
  status: string;
};

export type ReadyzResponseType = {
  status: string;
  version?: string;
};