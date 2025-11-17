export type Message = {
  key: string;
  thread: string;
  author?: string;
  role?: string; // e.g. "user" | "system"; defaults to "user" when omitted
  created_ts?: number;
  updated_ts?: number;
  body?: any;
  reply_to?: string;
  deleted?: boolean;
};

export type MessageCreateRequest = {
  body: any;
  reply_to?: string;
};

export type MessageUpdateRequest = {
  body: any;
};

export type MessageResponse = {
  message: Message;
};