import { useState } from 'react';
import { Form, Button, Typography, Toast } from '@douyinfe/semi-ui';

interface Props {
  requiresSetup: boolean;
  onLogin: (password: string) => Promise<void>;
  onSetup: (password: string) => Promise<void>;
}

export function LoginPage({ requiresSetup, onLogin, onSetup }: Props) {
  const [loading, setLoading] = useState(false);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');

  const handleSubmit = async () => {
    if (!password) { Toast.error('请输入密码'); return; }
    if (requiresSetup && password.length < 12) { Toast.error('管理员密码至少 12 个字符'); return; }
    if (requiresSetup && password !== confirm) { Toast.error('两次密码不一致'); return; }
    setLoading(true);
    try {
      if (requiresSetup) await onSetup(password);
      else await onLogin(password);
      Toast.success(requiresSetup ? '设置成功' : '登录成功');
    } catch (e: any) {
      Toast.error(e.message || '操作失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-shell">
      <div className="auth-panel">
        <Typography.Title heading={3} style={{ marginBottom: 24 }}>
          {requiresSetup ? '首次设置' : '登录'}
        </Typography.Title>
        {requiresSetup && (
          <Typography.Paragraph type="secondary" style={{ marginBottom: 24 }}>
            首次使用？创建管理员账号即可开始。
          </Typography.Paragraph>
        )}
        <Form onSubmit={handleSubmit}>
          <Form.Input
            label="密码"
            field="password"
            type="password"
            initValue={password}
            onChange={setPassword}
            placeholder={requiresSetup ? '至少 12 个字符' : '输入管理员密码'}
            rules={requiresSetup ? [{ required: true }, { min: 12, message: '管理员密码至少 12 个字符' }] : [{ required: true }]}
          />
          {requiresSetup && (
            <Form.Input
              label="确认密码"
              field="confirm"
              type="password"
              initValue={confirm}
              onChange={setConfirm}
              placeholder="再次输入密码"
              rules={[{ required: true }, { validator: (_: any, v: string) => v === password ? true : new Error('密码不一致') }]}
            />
          )}
          <Button htmlType="submit" type="primary" loading={loading} block style={{ marginTop: 12 }}>
            {requiresSetup ? '创建管理员' : '登录'}
          </Button>
        </Form>
      </div>
    </div>
  );
}
