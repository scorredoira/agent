# Document Relevance Evaluation Prompt

You are evaluating if a documentation file is relevant to a user's search query.

Search Query: "{{.SearchQuery}}"

File Information:
- File name: {{.FileName}}
- Directory path: {{.DirName}}
- Full path: {{.FilePath}}

Based ONLY on the file name and path, determine if this document is likely relevant to the search query.

Consider:
- Keywords in the filename that match the search topic
- Directory structure that indicates relevance
- Common abbreviations and variations
- IMPORTANT: Authentication, pagination, dates, and quickstart docs are relevant to ALL API operations
- IMPORTANT: Only exclude docs that are clearly about different business domains

Examples:
- Query "API authentication" + File "api_authentication.md" = RELEVANT
- Query "bookings API" + File "api_authentication.md" = RELEVANT (auth needed for any API)
- Query "bookings API" + File "api_quickstart.md" = RELEVANT (general API guidance)
- Query "bookings API" + File "api_pagination.md" = RELEVANT (pagination applies to all APIs)
- Query "bookings" + File "billing_overview.md" = NOT_RELEVANT (different business domain)
- Query "payments" + File "billing_overview.md" = RELEVANT (same business domain)
- Query "invoices" + File "bookings_cancel.md" = NOT_RELEVANT (different business domain)

Respond with exactly one word: "RELEVANT" or "NOT_RELEVANT"