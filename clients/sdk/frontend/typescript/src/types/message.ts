export type MessageType = {
  key: string;
  thread: string;
  author: string;
  created_ts?: number;
  updated_ts?: number;
  body?: any;
  deleted?: boolean;
};

export type MessageCreateRequestType = {
  body: any; // Any JSON serializable data (object, string, number, etc.)
};

export type MessageUpdateRequestType = {
  body: any; // Any JSON serializable data (object, string, number, etc.)
};

