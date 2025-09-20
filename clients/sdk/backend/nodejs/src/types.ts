/**
 * Message represents a single message stored in ProgressDB.
 */
export interface Message {
  id: string;
  thread: string;
  author: string;
  role?: string;
  ts: number;
  body?: any;
  reply_to?: string;
  deleted?: boolean;
  reactions?: Record<string,string>;
}

/**
 * Thread metadata stored in ProgressDB.
 */
export interface Thread {
  id: string;
  title: string;
  author: string;
  slug?: string;
  created_ts?: number;
  updated_ts?: number;
}
