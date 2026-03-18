// src/verify.ts
// Webhook signature verification for Node.js.
//
// Usage (Express):
//   import { verifyWebhook } from 'agentguard'
//
//   app.post('/webhooks/agentguard', express.raw({type: '*/*'}),
//     (req, res) => {
//       const valid = verifyWebhook(
//         req.body,
//         req.headers['x-agentguard-signature'] as string,
//         process.env.AGENTGUARD_WEBHOOK_SECRET!
//       )
//       if (!valid) return res.status(403).send('Forbidden')
//       const payload = JSON.parse(req.body)
//       // handle...
//     }
//   )

import { createHmac, timingSafeEqual } from 'crypto'

/**
 * Verify an AgentGuard webhook signature.
 *
 * @param body      - Raw request body (Buffer or string)
 * @param signature - Value of X-AgentGuard-Signature header
 * @param secret    - Your AGENTGUARD_WEBHOOK_SECRET
 * @returns         - true if signature is valid
 */
export function verifyWebhook(
  body:      Buffer | string,
  signature: string,
  secret:    string,
): boolean {
  if (!signature || !secret) return false

  // Strip "sha256=" prefix
  const sig = signature.startsWith('sha256=')
    ? signature.slice(7)
    : signature

  const expected = createHmac('sha256', secret)
    .update(body)
    .digest('hex')

  // Timing-safe comparison
  try {
    return timingSafeEqual(
      Buffer.from(expected, 'hex'),
      Buffer.from(sig,      'hex'),
    )
  } catch {
    // Buffer lengths differ = definitely invalid
    return false
  }
}
