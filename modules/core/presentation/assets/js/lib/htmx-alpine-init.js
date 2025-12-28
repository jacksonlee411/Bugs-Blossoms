// HTMX-Alpine.js Integration
//
// Alpine v3 already initializes newly inserted DOM via MutationObserver.
// Re-initializing Alpine manually after HTMX swaps can cause duplicated x-for
// clones and event handlers (especially when swaps are frequent).
//
// Keep this file as a no-op placeholder to preserve the existing `<script>` tag.
(function () {})();
