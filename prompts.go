package gaslit

// jsonInstruction tells an agent to reply with only a structured JSON object,
// including per-option confidences and an optional cross-examination challenge.
const jsonInstruction = `Respond with ONLY a valid JSON object (no markdown, no backticks, no extra prose):
{"answer": "<chosen option label, e.g., A>", "confidences": {"<label>": <float 0.0-1.0>, ...}, "reasoning": "<one sentence reasoning>", "challenge": {"agent_name": "<exact panelist name>", "prompt": "<the specific claim or weakness you want them confronted with>"}}
Every option label MUST be present in "confidences".
The "challenge" field is OPTIONAL — include it only if you want to nominate another panelist for cross-examination. Omit it entirely if you have no challenge to raise.`
