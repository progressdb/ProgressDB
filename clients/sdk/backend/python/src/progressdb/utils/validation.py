from typing import Dict, Any


def validate_pagination_query(query: Dict[str, Any]) -> None:
    """Validate that only one of anchor, before, after is specified."""
    ref_count = sum(1 for key in ['before', 'after', 'anchor'] if query.get(key))
    
    if ref_count > 1:
        raise ValueError('only one of anchor, before, after can be specified')