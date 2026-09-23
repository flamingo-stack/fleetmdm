import React from "react";

import FormField from "components/forms/FormField";

interface ITeamNameFieldProps {
  name: string;
}

const TeamNameField = ({ name }: ITeamNameFieldProps) => {
  return (
    <FormField label="Team" name="team_name">
      <p>{name}</p>
    </FormField>
  );
};

export default TeamNameField;

