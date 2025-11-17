// Query parameter types for frontend operations

export type ThreadListQueryType = {
  limit?: number;
  before?: string;
  after?: string;
  anchor?: string;
  sort_by?: 'created_ts' | 'updated_ts';
};

export type MessageListQueryType = {
  limit?: number;
  before?: string;
  after?: string;
  anchor?: string;
  sort_by?: 'created_ts' | 'updated_ts';
};