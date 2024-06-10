import classNames from "classnames";
import { Link } from "react-router-dom";
import { Shortcut } from "@/types/proto/api/v1/shortcut_service";
import { useStorage } from "@plasmohq/storage/hook";
import Icon from "./Icon";
import LinkFavicon from "./LinkFavicon";
import ShortcutActionsDropdown from "./ShortcutActionsDropdown";

interface Props {
  shortcut: Shortcut;
  className?: string;
  showActions?: boolean;
  alwaysShowLink?: boolean;
  onClick?: () => void;
}

const [shortcutPrefix] = useStorage<string>("shortcut_prefix", "s");
const ShortcutView = (props: Props) => {
  const { shortcut, className, showActions, alwaysShowLink, onClick } = props;

  return (
    <div
      className={classNames(
        "group w-full px-3 py-2 flex flex-row justify-start items-center border rounded-lg hover:bg-gray-100 dark:border-zinc-800 dark:hover:bg-zinc-800",
        className,
      )}
      onClick={onClick}
    >
      <div className={classNames("w-5 h-5 flex justify-center items-center overflow-clip shrink-0")}>
        <LinkFavicon url={shortcut.link} />
      </div>
      <div className="ml-2 w-full truncate">
        {shortcut.title ? (
          <>
            <span className="dark:text-gray-400">{shortcut.title}</span>
            <span className="text-gray-500">({shortcut.name})</span>
          </>
        ) : (
          <>
            <span className="dark:text-gray-400">{shortcut.name}</span>
          </>
        )}
      </div>
      <Link
        className={classNames(
          "hidden group-hover:block ml-1 w-6 h-6 p-1 shrink-0 rounded-lg bg-gray-200 dark:bg-zinc-900 hover:opacity-80",
          alwaysShowLink && "!block",
        )}
        to={`/${shortcutPrefix}/${shortcut.name}`}
        target="_blank"
        onClick={(e) => e.stopPropagation()}
      >
        <Icon.ArrowUpRight className="w-4 h-auto text-gray-400 shrink-0" />
      </Link>
      {showActions && (
        <div className="ml-1 flex flex-row justify-end items-center shrink-0" onClick={(e) => e.stopPropagation()}>
          <ShortcutActionsDropdown shortcut={shortcut} />
        </div>
      )}
    </div>
  );
};

export default ShortcutView;
