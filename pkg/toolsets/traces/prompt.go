package traces

const ServerPrompt = `
## Instructions for using the Tempo tools
Do not query across multiple instances unless specifically asked by the user.
Do not query across multiple tenants unless specifically asked by the user.
Ask the user which Tempo instance to query if the user did not specify a Tempo instance explicitly.
If the Tempo instance has multi-tenancy enabled: Ask the user which tenant to query if the user did not specify a tenant explicitly.
`
