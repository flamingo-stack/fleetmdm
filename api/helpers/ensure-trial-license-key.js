module.exports = {


  friendlyName: 'Ensure trial license key',


  description: 'Generate and persist a Fleet Premium trial license key for the given user if they do not already have one.',


  inputs: {

    user: {
      type: 'ref',
      description: 'The logged-in user record (this.req.me) to check and possibly update.',
      required: true,
    }

  },


  exits: {

    success: {
      outputDescription: 'The trial license key for this user, along with whether it is expired.',
      outputType: {
        trialLicenseKey: 'string',
        userHasExpiredTrialLicense: 'boolean',
      }
    }

  },


  fn: async function ({user}) {

    let userHasExpiredTrialLicense = false;
    let trialLicenseKey;

    if(user.fleetPremiumTrialLicenseKey) {
      if(user.fleetPremiumTrialLicenseKeyExpiresAt < Date.now()) {
        userHasExpiredTrialLicense = true;
      }
      trialLicenseKey = user.fleetPremiumTrialLicenseKey;
    } else {
      // If this user does not have a trial license key, generate a new one for them.
      let thirtyDaysFromNowAt = Date.now() + (1000 * 60 * 60 * 24 * 30);
      let trialLicenseKeyForThisUser = await sails.helpers.createLicenseKey.with({
        numberOfHosts: 10,
        organization: user.organization ? user.organization : 'Fleet Premium trial',
        expiresAt: thirtyDaysFromNowAt,
      });
      // Save the trial license key to the DB record for this user.
      await User.updateOne({id: user.id})
      .set({
        fleetPremiumTrialLicenseKey: trialLicenseKeyForThisUser,
        fleetPremiumTrialLicenseKeyExpiresAt: thirtyDaysFromNowAt,
      });
      trialLicenseKey = trialLicenseKeyForThisUser;
    }

    return {
      trialLicenseKey,
      userHasExpiredTrialLicense,
    };

  }


};
