import type { MouseEvent } from 'react';
import { Typography, Button, Toast } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';

export function CopyableUrl({ url, label }: { url: string; label?: string }) {
  const copy = async (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(url);
      } else {
        copyWithFallback(url);
      }
      Toast.success('已复制到剪贴板');
    } catch {
      try {
        copyWithFallback(url);
        Toast.success('已复制到剪贴板');
      } catch {
        Toast.error('复制失败');
      }
    }
  };
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, overflow: 'hidden' }}>
      {label && <Typography.Text type="secondary" style={{ flexShrink: 0 }}>{label}</Typography.Text>}
      <span style={{ fontFamily: 'monospace', fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }} title={url}>{url}</span>
      <Button icon={<IconCopy />} size="small" onClick={copy} style={{ flexShrink: 0 }} aria-label="复制代理地址" />
    </div>
  );
}

function copyWithFallback(text: string) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.top = '-9999px';
  textarea.style.left = '-9999px';
  document.body.appendChild(textarea);
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);
  const copied = document.execCommand('copy');
  document.body.removeChild(textarea);
  if (!copied) {
    throw new Error('copy command failed');
  }
}
