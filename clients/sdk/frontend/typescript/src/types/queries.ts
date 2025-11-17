// Query parameter types for frontend operations

export type ThreadListQueryType = {
  limit?: number; // 1-100 (service enforced)
  before?: string; // pagination anchor for newer threads
  after?: string;  // pagination anchor for older threads
  anchor?: string;  // specific pagination anchor
  sort_by?: 'created_ts' | 'updated_ts'; // sort field
};

export type MessageListQueryType = {
  limit?: number; // 1-100 (service enforced)
  before?: string; // pagination anchor for older messages
  after?: string;  // pagination anchor for newer messages
  anchor?: string;  // specific pagination anchor
  sort_by?: 'created_ts' | 'updated_ts'; // sort field
};