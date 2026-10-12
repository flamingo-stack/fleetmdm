import { ReactElement } from "react";
import { getErrorReason } from "interfaces/errors";

import {
  ADD_SOFTWARE_ERROR_PREFIX,
  DEFAULT_ADD_SOFTWARE_ERROR_MESSAGE,
  ensurePeriod,
  formatAlreadyAvailableInstallMessage,
} from "../../helpers";

// eslint-disable-next-line import/prefer-default-export
export const getErrorMessage = (e: unknown): string | ReactElement => {
  const reason = getErrorReason(e);

  if (reason.includes("find ID on the Play Store")) {
    return reason;
  }

  // software is already available for install
  if (reason.toLowerCase().includes("already")) {
    const alreadyAvailableMessage = formatAlreadyAvailableInstallMessage(
      reason
    );
    if (alreadyAvailableMessage) {
      return alreadyAvailableMessage;
    }
  }

  if (reason) {
    return `${ADD_SOFTWARE_ERROR_PREFIX} ${ensurePeriod(reason)}`;
  }

  return DEFAULT_ADD_SOFTWARE_ERROR_MESSAGE;
};
