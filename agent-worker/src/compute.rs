use std::net::{IpAddr, ToSocketAddrs};
use std::time::Duration;

use reqwest::redirect::Policy;
use serde_json::{json, Value};
use sha3::{Digest, Keccak256};

use crate::blockchain::BlockchainClient;
use crate::protocol::{resolve_service, ComputeRequest};

const MAX_PAGE_BYTES: usize = 256 * 1024;

pub async fn execute_service(
    request: &ComputeRequest,
    chain: &BlockchainClient,
) -> Result<(String, [u8; 32]), String> {
    let service = resolve_service(request)?;
    let output = match service {
        "web_brief" => web_brief(&request.prompt).await?,
        "chain_audit" | "hop_chain_audit" => chain_audit(&request.prompt, chain).await?,
        "deep_due_diligence" => {
            Err("deep_due_diligence is settled by the parent worker".to_string())?
        }
        "micro_call" => json!({
            "service": "micro_call",
            "echo": request.prompt
        }),
        _ => ai_synthesis(&request.prompt).await?,
    };
    let encoded = serde_json::to_vec(&output).map_err(|err| err.to_string())?;
    Ok((
        String::from_utf8_lossy(&encoded).into_owned(),
        keccak(&encoded),
    ))
}

async fn web_brief(prompt: &str) -> Result<Value, String> {
    let url = validate_public_url(prompt)?;
    let client = reqwest::Client::builder()
        .redirect(Policy::none())
        .timeout(Duration::from_secs(10))
        .build()
        .map_err(|err| err.to_string())?;
    let response = client
        .get(url.clone())
        .send()
        .await
        .map_err(|err| format!("fetch failed: {err}"))?;
    if !response.status().is_success() {
        return Err(format!("fetch returned {}", response.status()));
    }
    let bytes = response
        .bytes()
        .await
        .map_err(|err| format!("fetch body failed: {err}"))?;
    if bytes.len() > MAX_PAGE_BYTES {
        return Err("response exceeds 256 KB".to_string());
    }
    let html = String::from_utf8_lossy(&bytes);
    let title = capture(&html, "<title", "</title>");
    let description = meta_description(&html);
    let text = strip_tags(&html);
    let summary: String = text.chars().take(800).collect();
    Ok(json!({
        "service": "web_brief",
        "mode": "fetched",
        "url": url,
        "title": title,
        "description": description,
        "summary": summary,
    }))
}

async fn ai_synthesis(prompt: &str) -> Result<Value, String> {
    let key = std::env::var("LLM_API_KEY").unwrap_or_default();
    if key.trim().is_empty() {
        return Ok(json!({
            "service": "ai_synthesis",
            "mode": "deterministic_fallback",
            "fallback": true,
            "summary": fallback_brief(prompt),
        }));
    }
    let url = std::env::var("LLM_API_URL").unwrap_or_else(|_| {
        "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
            .to_string()
    });
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(25))
        .build()
        .map_err(|err| err.to_string())?;
    let text = if url.contains("generativelanguage.googleapis.com") {
        let endpoint = if url.contains('?') {
            format!("{url}&key={key}")
        } else {
            format!("{url}?key={key}")
        };
        let body = json!({
            "contents": [{ "parts": [{ "text": prompt }] }]
        });
        let response = client
            .post(endpoint)
            .json(&body)
            .send()
            .await
            .map_err(|err| format!("llm request failed: {err}"))?;
        let payload: Value = response
            .json()
            .await
            .map_err(|err| format!("llm response failed: {err}"))?;
        payload["candidates"][0]["content"]["parts"][0]["text"]
            .as_str()
            .unwrap_or("model returned no text")
            .to_string()
    } else {
        let response = client
            .post(url)
            .bearer_auth(key)
            .json(&json!({
                "model": std::env::var("LLM_MODEL").unwrap_or_else(|_| "gpt-4o-mini".to_string()),
                "messages": [{ "role": "user", "content": prompt }]
            }))
            .send()
            .await
            .map_err(|err| format!("llm request failed: {err}"))?;
        let payload: Value = response
            .json()
            .await
            .map_err(|err| format!("llm response failed: {err}"))?;
        payload["choices"][0]["message"]["content"]
            .as_str()
            .unwrap_or("model returned no text")
            .to_string()
    };
    Ok(json!({
        "service": "ai_synthesis",
        "mode": "llm",
        "fallback": false,
        "summary": text,
    }))
}

async fn chain_audit(prompt: &str, chain: &BlockchainClient) -> Result<Value, String> {
    let target = prompt.trim();
    if !target.starts_with("0x") || target.len() != 42 {
        return Err("chain_audit prompt must be a 20-byte hex address".to_string());
    }
    chain
        .inspect_address(target)
        .await
        .map_err(|err| err.to_string())
}

pub fn validate_public_url(raw: &str) -> Result<String, String> {
    let trimmed = raw.trim();
    let parsed = reqwest::Url::parse(trimmed).map_err(|err| format!("invalid url: {err}"))?;
    if parsed.scheme() != "http" && parsed.scheme() != "https" {
        return Err("only http and https URLs are accepted".to_string());
    }
    let host = parsed.host_str().ok_or("url has no host")?;
    if host.eq_ignore_ascii_case("localhost") {
        return Err("private hosts are blocked".to_string());
    }
    let mut blocked = false;
    if let Ok(ip) = host.parse::<IpAddr>() {
        blocked = !is_public_ip(ip);
    } else if let Ok(addrs) = (host, 0).to_socket_addrs() {
        for addr in addrs {
            if !is_public_ip(addr.ip()) {
                blocked = true;
            }
        }
    }
    if blocked {
        return Err("private hosts are blocked".to_string());
    }
    Ok(parsed.to_string())
}

fn is_public_ip(ip: IpAddr) -> bool {
    match ip {
        IpAddr::V4(v4) => {
            let octets = v4.octets();
            let cgnat = octets[0] == 100 && (octets[1] & 0b1100_0000) == 64;
            !v4.is_private()
                && !v4.is_loopback()
                && !v4.is_link_local()
                && !v4.is_broadcast()
                && !v4.is_documentation()
                && !v4.is_unspecified()
                && !cgnat
        }
        IpAddr::V6(v6) => {
            !v6.is_loopback()
                && !v6.is_unspecified()
                && !v6.is_unique_local()
                && !v6.is_unicast_link_local()
        }
    }
}

fn fallback_brief(prompt: &str) -> String {
    let mut hasher = Keccak256::new();
    hasher.update(prompt.as_bytes());
    let digest = hex::encode(hasher.finalize());
    format!(
        "Deterministic brief for \"{prompt}\". No LLM_API_KEY is configured, so this is not a model completion. Digest {digest}."
    )
}

fn capture(html: &str, start_tag: &str, end_tag: &str) -> String {
    let lower = html.to_ascii_lowercase();
    let start = match lower.find(start_tag) {
        Some(index) => index,
        None => return String::new(),
    };
    let after = match html[start..].find('>') {
        Some(index) => start + index + 1,
        None => return String::new(),
    };
    let end = match lower[after..].find(end_tag) {
        Some(index) => after + index,
        None => return String::new(),
    };
    html[after..end].trim().to_string()
}

fn meta_description(html: &str) -> String {
    let lower = html.to_ascii_lowercase();
    let marker = "name=\"description\"";
    let Some(index) = lower.find(marker) else {
        return String::new();
    };
    let window = &html[index..html.len().min(index + 400)];
    let Some(content) = window.to_ascii_lowercase().find("content=\"") else {
        return String::new();
    };
    let start = index + content + "content=\"".len();
    let rest = &html[start..];
    rest.split('"').next().unwrap_or("").trim().to_string()
}

fn strip_tags(html: &str) -> String {
    let mut out = String::new();
    let mut inside = false;
    for ch in html.chars() {
        match ch {
            '<' => inside = true,
            '>' => inside = false,
            _ if !inside => out.push(ch),
            _ => {}
        }
    }
    out.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn keccak(bytes: &[u8]) -> [u8; 32] {
    let mut hasher = Keccak256::new();
    hasher.update(bytes);
    hasher.finalize().into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn blocks_loopback_and_accepts_public_host_shape() {
        assert!(validate_public_url("http://127.0.0.1/secret").is_err());
        assert!(validate_public_url("http://localhost/admin").is_err());
        assert!(validate_public_url("http://192.168.1.8/").is_err());
        assert!(validate_public_url("file:///etc/passwd").is_err());
    }

    #[test]
    fn fallback_brief_is_stable() {
        let first = fallback_brief("escrow");
        let second = fallback_brief("escrow");
        assert_eq!(first, second);
        assert!(first.contains("Deterministic"));
    }
}
