module.exports = {


  friendlyName: 'View fleetctl preview',


  description: 'Display "fleetctl preview" page.',

  inputs: {
    start: {
      type: 'boolean',
      description: 'A boolean flag that will hide the "next steps" buttons on the page if set to true',
      defaultsTo: false,
    }
  },

  exits: {

    success: {
      viewTemplatePath: 'pages/fleetctl-preview'
    }

  },


  fn: async function ({start}) {

    let userHasTrialLicense = false;
    let trialLicenseKey;
    let userHasExpiredTrialLicense = false;

    if(this.req.me) {
      userHasTrialLicense = true;
      // Ensure this user has a (non-expired-check-aware) trial license key, generating one if needed.
      let trialLicenseInfo = await sails.helpers.ensureTrialLicenseKey.with({
        user: this.req.me,
      });
      trialLicenseKey = trialLicenseInfo.trialLicenseKey;
      userHasExpiredTrialLicense = trialLicenseInfo.userHasExpiredTrialLicense;
    }

    // Respond with view.
    return {
      hideNextStepsButtons: start,
      trialLicenseKey,
      userHasTrialLicense,
      userHasExpiredTrialLicense,
    };

  }


};

