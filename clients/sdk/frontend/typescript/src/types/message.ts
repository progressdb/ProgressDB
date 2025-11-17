export type Message = {
  key: string;
  thread: string;
  author?: string;
  created_ts?: number;
  updated_ts?: number;
  body?: any;
  deleted?: boolean;
};

export type MessageCreateRequest = {
  body: any;
};

export type MessageUpdateRequest = {
  body: any;
};

export type MessageResponse = {
  message: Message;
};