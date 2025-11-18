from typing import TypedDict, Optional


# Base query parameters (non-pagination)
class BaseQueryType(TypedDict, total=False):
    limit: Optional[int]  # 1-100 (service enforced)
    sort_by: Optional[str]  # 'created_ts' | 'updated_ts'

# Pagination query types - mutually exclusive
class BeforeQueryType(BaseQueryType):
    before: str
    # after and anchor are excluded by mutual exclusivity

class AfterQueryType(BaseQueryType):
    after: str
    # before and anchor are excluded by mutual exclusivity

class AnchorQueryType(BaseQueryType):
    anchor: str
    # before and after are excluded by mutual exclusivity

class NoPaginationQueryType(BaseQueryType):
    # before, after, anchor are excluded by mutual exclusivity
    pass

# Combined query types with mutual exclusivity enforced
ThreadListQueryType = BeforeQueryType | AfterQueryType | AnchorQueryType | NoPaginationQueryType
MessageListQueryType = BeforeQueryType | AfterQueryType | AnchorQueryType | NoPaginationQueryType