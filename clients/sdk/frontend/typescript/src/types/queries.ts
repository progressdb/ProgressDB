// Query parameter types for frontend operations

// Base query parameters (non-pagination)
type BaseQueryType = {
  limit?: number; // 1-100 (service enforced)
  sort_by?: 'created_ts' | 'updated_ts'; // sort field
};

// Pagination query types - mutually exclusive
type BeforeQueryType = BaseQueryType & {
  before: string;
  after?: never;
  anchor?: never;
};

type AfterQueryType = BaseQueryType & {
  after: string;
  before?: never;
  anchor?: never;
};

type AnchorQueryType = BaseQueryType & {
  anchor: string;
  before?: never;
  after?: never;
};

type NoPaginationQueryType = BaseQueryType & {
  before?: never;
  after?: never;
  anchor?: never;
};

// Combined query types with mutual exclusivity enforced
export type ThreadListQueryType = BeforeQueryType | AfterQueryType | AnchorQueryType | NoPaginationQueryType;
export type MessageListQueryType = BeforeQueryType | AfterQueryType | AnchorQueryType | NoPaginationQueryType;