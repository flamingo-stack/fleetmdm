import React, { useState } from "react";

import { ITeam } from "interfaces/team";
import { IEnrollSecret } from "interfaces/enroll_secret";

import Modal from "components/Modal";
import Button from "components/buttons/Button";
import InputField from "components/forms/fields/InputField";

interface ISecretEditorModalProps {
  selectedTeam: number;
  primoMode?: boolean;
  onSaveSecret: (newEnrollSecret: string) => void;
  teams: ITeam[];
  toggleSecretEditorModal: () => void;
  selectedSecret: IEnrollSecret | undefined;
  isUpdatingSecret: boolean;
}

const baseClass = "secret-editor-modal";

const randomSecretGenerator = () => {
  const randomChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
  const bytes = new Uint32Array(32);
  window.crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => randomChars[b % randomChars.length]).join(
    ""
  );
};

const SecretEditorModal = ({
  onSaveSecret,
  selectedTeam,
  primoMode = false,
  teams,
  toggleSecretEditorModal,
  selectedSecret,
  isUpdatingSecret,
}: ISecretEditorModalProps): JSX.Element => {
  const [enrollSecretString, setEnrollSecretString] = useState(
    selectedSecret ? selectedSecret.secret : randomSecretGenerator()
  );
  const [errors, setErrors] = useState<{ [key: string]: string }>({});

  const renderTeam = () => {
    const parsedSelectedTeam =
      typeof selectedTeam === "string"
        ? parseInt(selectedTeam, 10)
        : selectedTeam;

    if (parsedSelectedTeam === 0) {
      return { name: "Unassigned" };
    }
    return teams.find((team) => team.id === parsedSelectedTeam);
  };

  const onSecretChange = (value: string) => {
    if (value.length < 32) {
      setErrors({
        secret: "Secret",
      });
    } else {
      setErrors({});
    }
    setEnrollSecretString(value);
  };

  const onSaveSecretClick = () => {
    if (enrollSecretString.length < 32) {
      setErrors({
        secret: "Secret",
      });
    } else {
      setErrors({});
      onSaveSecret(enrollSecretString);
    }
  };

  return (
    <Modal
      onExit={toggleSecretEditorModal}
      onEnter={onSaveSecretClick}
      title={selectedSecret ? "Edit secret" : "Add secret"}
      className={baseClass}
    >
      <div className={baseClass}>
        <div className={`${baseClass}__description`}>
          Use these secret(s) to enroll hosts
          {primoMode || renderTeam()?.name === "Unassigned" ? (
            ""
          ) : (
            <>
              {" "}
              to <b>{renderTeam()?.name}</b>
            </>
          )}
          .
        </div>
        <div className={`${baseClass}__secret-wrapper`}>
          <InputField
            inputWrapperClass={`${baseClass}__secret-input`}
            name="osqueryd-secret"
            label="Secret"
            type="text"
            value={enrollSecretString}
            onChange={onSecretChange}
            error={errors.secret}
            helpText="Must contain at least 32 characters."
          />
        </div>
        <div className="modal-cta-wrap">
          <Button
            onClick={onSaveSecretClick}
            className="save-loading"
            isLoading={isUpdatingSecret}
          >
            Save
          </Button>
        </div>
      </div>
    </Modal>
  );
};

export default SecretEditorModal;
