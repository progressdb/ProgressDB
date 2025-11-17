import type { Thread } from './thread';
import type { Message } from './message';
import type { PaginationResponse } from './pagination';

export type KeyResponse = {
  key: string;
};

export type CreateThreadResponse = KeyResponse;
export type CreateMessageResponse = KeyResponse;
export type UpdateThreadResponse = KeyResponse;
export type UpdateMessageResponse = KeyResponse;
export type DeleteThreadResponse = KeyResponse;
export type DeleteMessageResponse = KeyResponse;

// Standardized response wrapper pattern
export type ApiResponse<T> = {
  data: T;
};

export type ThreadApiResponse = ApiResponse<Thread>;
export type MessageApiResponse = ApiResponse<Message>;
export type ThreadsListApiResponse = ApiResponse<Thread[]>;
export type MessagesListApiResponse = ApiResponse<Message[]>;

export type ThreadsListResponse = {
  threads: Thread[];
  pagination?: PaginationResponse;
};

export type MessagesListResponse = {
  thread?: string;
  messages: Message[];
  pagination?: PaginationResponse;
};

// Health check response types
export type HealthzResponse = {
  status: string;
};

export type ReadyzResponse = {
  status: string;
  version?: string;
};