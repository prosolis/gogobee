import logger from "./logger";

interface LoginResponse {
  access_token: string;
  device_id: string;
  user_id: string;
}

/**
 * Log in to the homeserver with a password and return a fresh access token.
 * Reuses the existing device_id when provided so the crypto store stays valid.
 */
export async function loginWithPassword(
  homeserverUrl: string,
  userId: string,
  password: string,
  deviceId?: string
): Promise<LoginResponse> {
  const localpart = userId.split(":")[0].replace(/^@/, "");

  const body: Record<string, string> = {
    type: "m.login.password",
    user: localpart,
    password,
  };
  if (deviceId) {
    body.device_id = deviceId;
  }

  const res = await fetch(`${homeserverUrl}/_matrix/client/v3/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Login failed (${res.status}): ${text}`);
  }

  const data = (await res.json()) as LoginResponse;
  logger.info(`Obtained new access token for device ${data.device_id}`);
  return data;
}

/**
 * Check whether the current access token is still valid.
 */
export async function isTokenValid(homeserverUrl: string, accessToken: string): Promise<boolean> {
  try {
    const res = await fetch(`${homeserverUrl}/_matrix/client/v3/account/whoami`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    return res.ok;
  } catch {
    return false;
  }
}
