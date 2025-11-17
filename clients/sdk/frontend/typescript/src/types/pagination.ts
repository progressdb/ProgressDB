export type PaginationResponseType = {
  before_anchor: string; // Use this to get previous page (pass as 'after' parameter)
  after_anchor: string;  // Use this to get next page (pass as 'before' parameter)
  has_before: boolean;   // True if there are items before before_anchor (previous page exists)
  has_after: boolean;    // True if there are items after after_anchor (next page exists)
  count: number;        // Number of items returned in this page
  total: number;        // Total number of items available
};