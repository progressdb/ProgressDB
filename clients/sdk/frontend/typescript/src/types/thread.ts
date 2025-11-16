export type Thread = {
  key: string;
  title?: string;
  slug?: string;
  created_ts?: number;
  updated_ts?: number;
  author?: string;
  deleted?: boolean;
  kms?: KMSMeta;
};

export type ThreadCreateRequest = {
  title: string;
  slug?: string;
};

export type ThreadUpdateRequest = {
  title?: string;
  slug?: string;
};

export type ThreadResponse = {
  thread: Thread;
};

export type KMSMeta = {
  key_id?: string;
  wrapped_dek?: string;
  kek_id?: string;
  kek_version?: string;
};