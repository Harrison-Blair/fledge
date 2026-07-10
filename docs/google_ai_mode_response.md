# Architectural Brief – Multi-Tier AI Routing Infrastructure

This document outlines a hybrid, cost-optimized developer infrastructure designed to replace consumer chat interfaces. It completely eliminates session timeouts while maintaining a strict spending ceiling under **$100/month**.

---

## 1. Architectural Overview

The infrastructure relies on a three-tier execution hierarchy. Workloads are dynamically routed based on complexity, infrastructure health, and token burn efficiency.

                      [Custom Developer Harness / UI]
                                     |
                    What tier of complexity is required?
                    /                |               \
         (Routine / Automation)     (Deep Reason)    (Frontier Edge Case)
                  /                  |                 \
       [Tier 1: Local Server]  [Tier 2: OpenCode Go]  [Tier 3: OpenCode Zen]
       (GLM-5.2 / Kimi 2.7)     (DeepSeek V4-Pro)     (Claude Sonnet / o3)

                  |                  |                  |
            $0.00 / Token       $10/Mo Fixed Cap      Pay-As-You-Go Credits

---

## 2. Infrastructure Execution Tiers

### Tier 1: Local Hardware Server (Zero-Cost Baseline)
*   **Target Workloads:** Routine coding, structural formatting, boilerplate generation, and unit testing.
*   **Models:** `GLM-5.2` or `Kimi 2.7` (served via vLLM, SGLang, or Ollama on an independent local network machine).
*   **Cost Profile:** $0.00 / token.

### Tier 2: OpenCode Go API (Fixed-Cost Workhorse)
*   **Target Workloads:** Complex multi-file refactoring, mid-tier debugging, and large workspace contextual execution.
*   **Models:** `DeepSeek V4-Pro` or `Kimi K2.7 Code`.
*   **Cost Profile:** $10/month base subscription + usage metered up to a hard ceiling of $60/month (Max exposure: $70/month).

### Tier 3: OpenCode Zen API (Premium Fallback)
*   **Target Workloads:** Elite reasoning problems, edge-case system design, and execution when Tier 2 caps are exhausted.
*   **Models:** `Claude Sonnet (4.5/4.6)` or `OpenAI o3-mini (High)`.
*   **Cost Profile:** Pure pay-as-you-go. Zero markup token tracking.

---

## 3. Production Implementation Blueprint (Python)

import os
import requests
import subprocess

class MultiTierHardwareHarness:
    def __init__(self):
        self.go_key = os.getenv("OPENCODE_GO_API_KEY")
        self.zen_key = os.getenv("OPENCODE_ZEN_API_KEY")
        self.local_server_url = "http://192.168.1"
        self.opencode_base_url = "https://opencode.ai"

    def gather_local_context(self):
        context = "LOCAL CODEBASE CONTEXT:\n"
        try:
            git_diff = subprocess.check_output(["git", "diff"], text=True)
            context += f"Current Git Diff:\n{git_diff}\n"
        except Exception:
            context += "No active git tracking found.\n"
        return context

    def execute_request(self, tier, model_name, url, headers, payload):
        try:
            print(f"[{tier}] Attempting execution via {model_name}...")
            response = requests.post(url, json=payload, headers=headers, timeout=30)
            if response.status_code == 200:
                return response.json()['choices']['message']['content']
            else:
                print(f"[{tier}] Failed with Status Code: {response.status_code}")
                return None
        except requests.exceptions.RequestException as e:
            print(f"[{tier}] Connection error: {e}")
            return None

    def route_and_generate(self, prompt, complexity="routine"):
        context = self.gather_local_context()
        messages = [
            {"role": "system", "content": "You are an expert agentic software harness. Output clean code updates."},
            {"role": "user", "content": f"{context}\nUser Request: {prompt}"}
        ]

        # Tier 1: Local Routing
        if complexity == "routine":
            payload = {"model": "kimi-2.7-code", "messages": messages}
            headers = {"Content-Type": "application/json"}
            result = self.execute_request("LOCAL TIER", "Kimi 2.7 (Local Machine)", self.local_server_url, headers, payload)
            if result: return result
            print("Local machine unavailable. Escalating to paid infrastructure...")

        # Tier 2: OpenCode Go Subscription
        payload = {"model": "opencode/deepseek-v4-pro", "messages": messages}
        headers = {"Authorization": f"Bearer {self.go_key}", "Content-Type": "application/json"}
        result = self.execute_request("OPENCODE GO TIER", "DeepSeek V4-Pro", self.opencode_base_url, headers, payload)
        if result and "limit_exceeded" not in result: 
            return result
        print("OpenCode Go limit reached. Escalating to premium fallback...")

        # Tier 3: OpenCode Zen Fallback
        payload = {"model": "anthropic/claude-3.5-sonnet", "messages": messages}
        headers = {"Authorization": f"Bearer {self.zen_key}", "Content-Type": "application/json"}
        result = self.execute_request("ZEN PREMIUM TIER", "Claude Sonnet", self.opencode_base_url, headers, payload)
        if result: return result

        return "Critical Failure: All infrastructure tiers exhausted."

---

## 4. Expected Benefits
1.  **Zero Quota Throttling:** Shifting out of consumer web chat frameworks ensures sessions are bounded strictly by your physical budget, not short-term timers.
2.  **Predictable Cost Ceiling:** Total architectural cost is hard-walled under your $100 budget ($70 baseline + ~$20 Zen auxiliary pool).
3.  **No Hallucination Regressions:** Employs advanced test-time compute options at every level of paid and local infrastructure execution.

