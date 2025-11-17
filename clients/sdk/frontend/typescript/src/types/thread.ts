export type ThreadType = {
  key: string;
  title?: string;
  created_ts?: number;
  updated_ts?: number;
  author?: string;
  deleted?: boolean;
  kms?: KMSMetaType;
};

export type ThreadCreateRequestType = {
  title: string;
};

export type ThreadUpdateRequestType = {
  title?: string;
};

export type ThreadResponseType = {
  thread: ThreadType;
};

export type KMSMetaType = {
  key_id?: string;
  wrapped_dek?: string;
  kek_id?: string;
  kek_version?: string;
};