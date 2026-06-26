import { SideSheet, Modal } from '@douyinfe/semi-ui';
import { useMediaQuery } from '../hooks/useMediaQuery';

interface Props {
  title: string; visible: boolean; onClose: () => void;
  children: React.ReactNode; footer?: React.ReactNode; width?: number;
}

export function AdaptivePanel({ title, visible, onClose, children, footer, width = 480 }: Props) {
  const isMobile = useMediaQuery('(max-width: 767px)');
  const bodyStyle = { overflowY: 'auto' as const, minHeight: 0 };
  if (isMobile) {
    return (
      <Modal
        className="adaptive-panel-mobile"
        title={title}
        visible={visible}
        onCancel={onClose}
        footer={footer}
        fullScreen
        bodyStyle={bodyStyle}
      >
        {children}
      </Modal>
    );
  }
  return <SideSheet title={title} visible={visible} onCancel={onClose} width={width} footer={footer} bodyStyle={bodyStyle}>{children}</SideSheet>;
}
