// Query parameter types for frontend operations

export type ThreadListQuery = {
  limit?: number;
  before?: string;
  after?: string;
  anchor?: string;
  sort_by?: 'created_ts' | 'updated_ts';
};

export type MessageListQuery = {
  limit?: number;
  before?: string;
  after?: string;
  anchor?: string;
  sort_by?: 'created_ts' | 'updated_ts';
};