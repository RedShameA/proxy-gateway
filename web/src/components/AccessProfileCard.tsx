import type { CSSProperties, KeyboardEvent } from 'react';
import { Card, Tag, Typography } from '@douyinfe/semi-ui';
import { formatRelativeTime, pathSummaryText, profileIssueSummary, profileStateColor, profileStateLabel, profileTypeLabel } from '../display';
import type { AccessProfileSummary } from '../types';

const { Text } = Typography;

type AccessProfileCardProps = {
  profile: AccessProfileSummary;
  onClick?: () => void;
  style?: CSSProperties;
  className?: string;
};

export function AccessProfileCard({ profile, onClick, style, className }: AccessProfileCardProps) {
  const issue = profileIssueSummary(profile);
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!onClick) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onClick();
    }
  };

  return (
    <div
      className={className}
      style={{ cursor: onClick ? 'pointer' : 'default', height: '100%', ...style }}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={handleKeyDown}
    >
      <Card style={{ height: '100%', overflow: 'hidden' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <Text strong style={{ fontSize: 15, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: 1 }}>{profile.name}</Text>
          <div style={{ display: 'flex', gap: 4, flexShrink: 0 }}>
            <Tag size="small">{profileTypeLabel(profile.type)}</Tag>
            <Tag color={profileStateColor(profile.state) as any} size="small">{profileStateLabel(profile.state)}</Tag>
          </div>
        </div>
        <Text size="small" type="secondary" style={{ display: 'block', marginBottom: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          策略标识: {profile.profile_identifier}
        </Text>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4, minWidth: 0 }}>
          <Text size="small" type="secondary" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', minWidth: 0, flex: '1 1 auto' }}>
            {profile.current_path ? pathSummaryText(profile.current_path) : '无可用路径'}
          </Text>
          {issue && (
            <Text
              size="small"
              type={(profile.state === 'degraded' ? 'warning' : 'danger') as any}
              ellipsis={{ showTooltip: true }}
              style={{ maxWidth: '48%', minWidth: 0, flex: '0 1 auto', textAlign: 'right' }}
            >
              {issue}
            </Text>
          )}
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between' }}>
          <Text size="small" type="secondary">凭证 {profile.enabled_proxy_credentials_count}/{profile.proxy_credentials_count}</Text>
          <Text size="small" type="secondary">{formatRelativeTime(profile.last_evaluated_at)}</Text>
        </div>
      </Card>
    </div>
  );
}
