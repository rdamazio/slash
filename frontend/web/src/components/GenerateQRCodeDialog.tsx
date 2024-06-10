import { Button, Modal, ModalDialog } from "@mui/joy";
import { QRCodeCanvas } from "qrcode.react";
import { useRef } from "react";
import { toast } from "react-hot-toast";
import { useTranslation } from "react-i18next";
import { absolutifyLink } from "@/helpers/utils";
import { Shortcut } from "@/types/proto/api/v1/shortcut_service";
import { useStorage } from "@plasmohq/storage/hook";
import Icon from "./Icon";

interface Props {
  shortcut: Shortcut;
  onClose: () => void;
}

const GenerateQRCodeDialog: React.FC<Props> = (props: Props) => {
  const { shortcut, onClose } = props;
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [shortcutPrefix] = useStorage<string>("shortcut_prefix", "s");
  const shortcutLink = absolutifyLink(`/${shortcutPrefix}/${shortcut.name}`);

  const handleCloseBtnClick = () => {
    onClose();
  };

  const handleDownloadQRCodeClick = () => {
    const canvas = containerRef.current?.querySelector("canvas");
    if (!canvas) {
      toast.error("Failed to get QR code canvas");
      return;
    }

    const link = document.createElement("a");
    link.download = `${shortcut.title || shortcut.name}-qrcode.png`;
    link.href = canvas.toDataURL();
    link.click();
    handleCloseBtnClick();
  };

  return (
    <Modal open={true}>
      <ModalDialog>
        <div className="flex flex-row justify-between items-center w-64">
          <span className="text-lg font-medium">QR Code</span>
          <Button variant="plain" onClick={handleCloseBtnClick}>
            <Icon.X className="w-5 h-auto text-gray-600" />
          </Button>
        </div>
        <div>
          <div ref={containerRef} className="w-full flex flex-row justify-center items-center mt-2 mb-6">
            <QRCodeCanvas value={shortcutLink} size={180} bgColor={"#ffffff"} fgColor={"#000000"} includeMargin={false} level={"L"} />
          </div>
          <div className="w-full flex flex-row justify-center items-center px-4">
            <Button className="w-full" color="neutral" onClick={handleDownloadQRCodeClick}>
              <Icon.Download className="w-4 h-auto mr-1" />
              {t("common.download")}
            </Button>
          </div>
        </div>
      </ModalDialog>
    </Modal>
  );
};

export default GenerateQRCodeDialog;
