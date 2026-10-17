// OPENFRAME(managed-queries): read-only indicator; the flag itself is forced in queryAPI.create — openframe/docs/managed-queries.md
import React from "react";

import Checkbox from "components/forms/fields/Checkbox";

const noop = () => undefined;

const OpenframeManagedOption = (): JSX.Element => (
  <Checkbox
    name="openframeManaged"
    onChange={noop}
    value
    disabled
    helpText="Reports created here are always managed by OpenFrame."
  >
    OpenFrame managed
  </Checkbox>
);

export default OpenframeManagedOption;
