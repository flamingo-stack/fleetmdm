module.exports = {


  friendlyName: 'Deliver expired local trial emails',


  description: 'A script designed to be run daily, that sends emails to Fleetdm.com users whose Fleet premium trial expired in the past 24 hours.',


  fn: async function () {

    sails.log('Running custom shell script... (`sails run deliver-expired-local-trial-emails`)');

    let nowAt = Date.now();
    // Use the last recorded run's timestamp (if available) as the start of the query window,
    // so that delayed, skipped, or double-run scripts do not cause expired trial users to be
    // missed. Falls back to 24 hours ago if this script has not recorded a last run yet.
    let lastRunAt = await sails.helpers.flow.build(async ()=>{
      let deliverExpiredLocalTrialEmailsScript = await Script.findOne({identifier: 'deliver-expired-local-trial-emails'});
      return deliverExpiredLocalTrialEmailsScript ? deliverExpiredLocalTrialEmailsScript.lastRanAt : undefined;
    });
    let oneDayAgoAt = nowAt - (1000 * 60 * 60 * 24);
    let queryWindowStartAt = lastRunAt || oneDayAgoAt;

    // Build a list of users with a local Fleet Premium trial that has expired since this script last ran
    // (or in the past 24 hours, if this is the first time this script has run.)
    let usersWithRecentlyExpiredLocalTrials = await User.find({
      fleetPremiumTrialType: 'local trial',
      fleetPremiumTrialLicenseKeyExpiresAt: {
        '>=': queryWindowStartAt,
        '<': nowAt,
      },
    });


    for(let expiredTrialUser of usersWithRecentlyExpiredLocalTrials) {
      if(!expiredTrialUser.fleetPremiumTrialEmailSentAt) {

        // Send an "Your fleet trial has ended" email to the user.
        await sails.helpers.sendTemplateEmail.with({
          to: expiredTrialUser.emailAddress,
          from: sails.config.custom.fromEmailAddress,
          fromName: sails.config.custom.fromName,
          subject: 'Your Fleet trial has ended',
          template: 'email-fleet-premium-local-trial-ended',
          layout: 'layout-nurture-email',
          templateData: {
            firstName: expiredTrialUser.firstName,
          },
          ensureAck: true,
        }).tolerate((err)=>{
          sails.log.warn(`When sending an email to a user with a newly expired Fleet Premium local trial (email: ${expiredTrialUser.emailAddress}) an error occured. Full error: ${require('util').inspect(err)}`);
          return;
        });
        // Update the fleetPremiumTrialEmailSentAt for this user.
        await User.updateOne({id: expiredTrialUser.id}).set({fleetPremiumTrialEmailSentAt: Date.now()});

      }

    }

    // Persist a watermark of this run's timestamp, so the next run's query window starts
    // where this one left off (instead of assuming exactly 24 hours have passed).
    await Script.updateOne({identifier: 'deliver-expired-local-trial-emails'})
    .set({lastRanAt: nowAt})
    .tolerate(async (err)=>{
      sails.log.warn(`When updating the lastRanAt watermark for deliver-expired-local-trial-emails, an error occured (this may mean the Script model/record does not exist yet). Full error: ${require('util').inspect(err)}`);
      return;
    });

    sails.log(`Sent expired trial emails for ${usersWithRecentlyExpiredLocalTrials.length} user(s).`);

  }


};

