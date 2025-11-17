export type { UserSignature, GetUserSignature, ProgressProviderProps, ProgressClientContextValue } from './provider';

// Re-export types from JS SDK for convenience
export type { 
  MessageType, 
  ThreadType, 
  ThreadCreateRequestType, 
  ThreadUpdateRequestType, 
  MessageCreateRequestType, 
  MessageUpdateRequestType,
  ThreadListQueryType,
  MessageListQueryType,
  PaginationResponseType,
  ThreadsListResponseType,
  MessagesListResponseType,
  ThreadResponseType,
  MessageResponseType,
  KeyResponseType,
  SDKOptionsType,
  ApiErrorResponseType
} from '@progressdb/js';