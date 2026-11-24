export async function copyText(text: string): Promise<boolean> {
  const value = text.trimEnd();
  if (!value) return false;
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    try {
      const node = document.createElement('textarea');
      node.value = value;
      node.setAttribute('readonly', '');
      node.style.position = 'fixed';
      node.style.left = '-9999px';
      document.body.appendChild(node);
      node.select();
      const ok = document.execCommand('copy');
      node.remove();
      return ok;
    } catch {
      return false;
    }
  }
}
