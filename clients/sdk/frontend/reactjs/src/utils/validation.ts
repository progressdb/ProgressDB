export function validatePaginationQuery(query: { before?: string; after?: string; anchor?: string }): void {
  const refCount = [query.before, query.after, query.anchor].filter(Boolean).length;
  
  if (refCount > 1) {
    throw new Error('only one of anchor, before, after can be specified');
  }
}