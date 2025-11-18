from typing import TypedDict, Optional


class PaginationResponseType(TypedDict):
    before_anchor: str  # Use this to get previous page (pass as 'after' parameter)
    after_anchor: str   # Use this to get next page (pass as 'before' parameter)
    has_before: bool    # True if there are items before before_anchor (previous page exists)
    has_after: bool     # True if there are items after after_anchor (next page exists)
    count: int         # Number of items returned in this page
    total: int         # Total number of items available