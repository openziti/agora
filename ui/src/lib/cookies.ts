const csrfCookieName = 'agora-csrf';
const localUserKey = 'agora.user';

export function getCsrfToken(): string | undefined {
  if (typeof document === 'undefined') {
    return undefined;
  }

  const match = document.cookie
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${csrfCookieName}=`));

  if (!match) {
    return undefined;
  }

  return decodeURIComponent(match.slice(csrfCookieName.length + 1));
}

export function clearLocalUser() {
  if (typeof localStorage === 'undefined') {
    return;
  }

  localStorage.removeItem(localUserKey);
}

export function setLocalUserEmail(email: string) {
  if (typeof localStorage === 'undefined') {
    return;
  }

  localStorage.setItem(localUserKey, email);
}

export function getLocalUserEmail(): string | undefined {
  if (typeof localStorage === 'undefined') {
    return undefined;
  }

  return localStorage.getItem(localUserKey) ?? undefined;
}
