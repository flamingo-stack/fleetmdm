import React from "react";
import classnames from "classnames";

import CustomLink from "components/CustomLink";
import Icon from "components/Icon";
import Graphic from "components/Graphic";
import { Padding } from "styles/var/padding";

const baseClass = "data-error";

interface IDataErrorProps {
  /** the description text displayed under the header */
  description?: string;
  /** Excludes the link that asks user to create an issue. Defaults to `false` */
  excludeIssueLink?: boolean;
  children?: React.ReactNode;
  /**
   * Sets the vertical padding for the component.
   * **Recommended values:**
   * - For card-level components, use "pad-large" `24px`.
   * - For page-level components, use "pad-xxxlarge"`80px`.
   * These values help maintain consistent spacing across the application.
   */
  verticalPaddingSize?: Padding;
  className?: string;
  /** Flag to use the updated DataError design */
  useNew?: boolean;
  /** Overrides something gone wrong line with description text to condense error onto one line */
  singleCustomLine?: boolean;
  /** Centers the component within its parent */
  selfCenter?: boolean;
}

const DEFAULT_DESCRIPTION = "Refresh the page or log in again.";

const getVerticalPaddingClass = (verticalPaddingSize?: Padding) =>
  verticalPaddingSize && `${baseClass}__vertical-${verticalPaddingSize}`;

const renderFileIssueLink = (
  wrapperClassName: string,
  copy: string
) => (
  <div className={wrapperClassName}>
    {copy}&nbsp;
    <CustomLink
      url="https://github.com/fleetdm/fleet/issues/new/choose"
      text="file an issue"
      newTab
    />
  </div>
);

const DataError = ({
  description = DEFAULT_DESCRIPTION,
  excludeIssueLink = false,
  children,
  verticalPaddingSize,
  className,
  useNew = false,
  singleCustomLine = false,
  selfCenter = false,
}: IDataErrorProps): JSX.Element => {
  const classes = classnames(baseClass, className, {
    [`${baseClass}--self-center`]: selfCenter,
  });

  if (singleCustomLine) {
    return (
      <div className={classes}>
        <div
          className={`${baseClass}__inner ${getVerticalPaddingClass(
            verticalPaddingSize
          )}`}
        >
          <div className={`${baseClass}__content`}>
            <span
              className={`${baseClass}__header ${baseClass}__header--single-line`}
            >
              <Icon name="error" />
              {description}
            </span>
          </div>
        </div>
      </div>
    );
  }

  if (useNew) {
    return (
      <div className={classes}>
        <div
          className={`${baseClass}__inner-new ${getVerticalPaddingClass(
            verticalPaddingSize
          )}`}
        >
          <Graphic name="data-error" />
          <div className={`${baseClass}__header`}>
            Something&apos;s gone wrong.
          </div>
          {children || (
            <>
              <div className={`${baseClass}__description`}>
                Refresh to try again.
              </div>
              {!excludeIssueLink &&
                renderFileIssueLink(
                  `${baseClass}__file-issue`,
                  "If this keeps happening please"
                )}
            </>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={classes}>
      <div
        className={`${baseClass}__inner ${getVerticalPaddingClass(
          verticalPaddingSize
        )}`}
      >
        <div className={`${baseClass}__content`}>
          <span className={`${baseClass}__header`}>
            <Icon name="error" />
            Something&apos;s gone wrong.
          </span>

          <>
            {children || (
              <>
                {description && (
                  <span className={`${baseClass}__description`}>
                    {description}
                  </span>
                )}
                {!excludeIssueLink &&
                  renderFileIssueLink(
                    `${baseClass}__file-issue`,
                    "If this keeps happening, please"
                  )}
              </>
            )}
          </>
        </div>
      </div>
    </div>
  );
};

export default DataError;
